// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mutator

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime/serializer/json"

	"github.com/gardener/gardener-extension-provider-alicloud/pkg/alicloud"
	alicloudclient "github.com/gardener/gardener-extension-provider-alicloud/pkg/alicloud/client"
	api "github.com/gardener/gardener-extension-provider-alicloud/pkg/apis/alicloud"
	"github.com/gardener/gardener-extension-provider-alicloud/pkg/apis/alicloud/helper"
	apisalicloudv1alpha1 "github.com/gardener/gardener-extension-provider-alicloud/pkg/apis/alicloud/v1alpha1"
	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	corev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/controllerutils"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ShootMutatorName is the shoots mutator webhook name.
	ShootMutatorName = "shoots.mutator"
	// MutatorPath is the mutator webhook path.
	MutatorPath = "/webhooks/mutate"
)

// NewShootMutator returns a new instance of a shoot mutator.
func NewShootMutator() extensionswebhook.Mutator {
	alicloudclientFactory := alicloudclient.NewClientFactory()
	return NewShootMutatorWithDeps(alicloudclientFactory)
}

// NewShootMutatorWithDeps with parameter returns a new instance of a shoot mutator.
func NewShootMutatorWithDeps(alicloudclientFactory alicloudclient.ClientFactory) extensionswebhook.Mutator {
	return &shootMutator{alicloudClientFactory: alicloudclientFactory}
}

type shootMutator struct {
	client                client.Client
	apiReader             client.Reader
	codec                 runtime.Codec
	alicloudClientFactory alicloudclient.ClientFactory
}

// InjectScheme injects the given scheme into the mutator.
func (s *shootMutator) InjectScheme(scheme *runtime.Scheme) error {
	codecFactory := serializer.NewCodecFactory(scheme, serializer.EnableStrict)
	decoder := codecFactory.UniversalDecoder()
	serializer := json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme, scheme, json.SerializerOptions{})
	s.codec = runtime.NewCodec(serializer, decoder)
	return nil
}

// InjectClient injects the given client into the mutator.
func (s *shootMutator) InjectClient(client client.Client) error {
	s.client = client
	return nil
}

// InjectAPIReader injects the given apiReader into the mutator.
func (s *shootMutator) InjectAPIReader(apiReader client.Reader) error {
	s.apiReader = apiReader
	return nil
}

func (s *shootMutator) Mutate(ctx context.Context, new, old client.Object) error {
	shoot, ok := new.(*corev1beta1.Shoot)
	if !ok {
		return fmt.Errorf("wrong object type %T", new)
	}

	if old != nil {
		oldShoot, ok := old.(*corev1beta1.Shoot)
		if !ok {
			return fmt.Errorf("wrong object type %T for old object", old)
		}
		return s.mutateShootUpdate(ctx, shoot, oldShoot)
	} else {
		return s.mutateShootCreation(ctx, shoot)
	}

}

func (s *shootMutator) mutateShootCreation(ctx context.Context, shoot *corev1beta1.Shoot) error {
	logger.Info("Starting Shoot Creation Mutation")

	err := s.mutateControlPlaneConfigForCreate(shoot)
	if err != nil {
		return err
	}

	for _, worker := range shoot.Spec.Provider.Workers {
		if err := s.setDefaultForEncryptedDisk(ctx, shoot, &worker); err != nil {
			return err
		}
	}

	return nil
}

func (s *shootMutator) setDefaultForEncryptedDisk(ctx context.Context, shoot *corev1beta1.Shoot, worker *corev1beta1.Worker) error {
	imageName := worker.Machine.Image.Name
	imageVersion := worker.Machine.Image.Version
	architecture := worker.Machine.Architecture
	logger.Info("Check ImageName: " + imageName + "; ImageVesion: " + *imageVersion)
	if worker.DataVolumes != nil {
		for i := range worker.DataVolumes {
			volume := &worker.DataVolumes[i]
			if volume.Encrypted == nil {
				logger.Info("Set encrypted disk by default for data disk")
				volume.Encrypted = pointer.BoolPtr(true)
			}
		}
	}
	if worker.Volume != nil && worker.Volume.Encrypted == nil {
		//don't set encrypted disk by default if image is system image
		isCustomizeImage, err := s.isCustomizedImage(ctx, shoot, imageName, imageVersion, architecture)
		if err != nil {
			return err
		}
		if !isCustomizeImage {
			return nil
		}
		logger.Info("Customized Image is used and we set encrypted disk by default for system disk")
		worker.Volume.Encrypted = pointer.BoolPtr(true)
	}
	return nil
}

func (s *shootMutator) isCustomizedImage(ctx context.Context, shoot *corev1beta1.Shoot, imageName string, imageVersion, architecture *string) (bool, error) {
	cloudProfile := shoot.Spec.CloudProfileName
	region := shoot.Spec.Region
	logger.Info("Checking in cloudProfie", "CloudProfile", cloudProfile, "Region", region)
	imageId, err := s.getImageId(ctx, imageName, region, cloudProfile, imageVersion, architecture)
	if err != nil || imageId == "" {
		return false, err
	}
	logger.Info("Got ImageID", "ImageID", imageId)
	isOwnedByAli, err := s.isOwnedbyAliCloud(ctx, shoot, imageId, region)
	return !isOwnedByAli, err
}

func (s *shootMutator) isOwnedbyAliCloud(ctx context.Context, shoot *corev1beta1.Shoot, imageId string, region string) (bool, error) {
	var (
		secretBinding    = &corev1beta1.SecretBinding{}
		secretBindingKey = kutil.Key(shoot.Namespace, shoot.Spec.SecretBindingName)
	)
	if err := kutil.LookupObject(ctx, s.client, s.apiReader, secretBindingKey, secretBinding); err != nil {
		return false, err
	}

	var (
		secret    = &corev1.Secret{}
		secretRef = secretBinding.SecretRef.Name
		secretKey = kutil.Key(secretBinding.SecretRef.Namespace, secretRef)
	)
	if err := s.apiReader.Get(ctx, secretKey, secret); err != nil {
		return false, err
	}
	accessKeyID, ok := secret.Data[alicloud.AccessKeyID]
	if !ok {
		return false, fmt.Errorf("missing %q field in secret %s", alicloud.AccessKeyID, secretRef)
	}
	accessKeySecret, ok := secret.Data[alicloud.AccessKeySecret]
	if !ok {
		return false, fmt.Errorf("missing %q field in secret %s", alicloud.AccessKeySecret, secretRef)
	}
	shootECSClient, err := s.alicloudClientFactory.NewECSClient(region, string(accessKeyID), string(accessKeySecret))
	if err != nil {
		return false, err
	}
	if exist, err := shootECSClient.CheckIfImageExists(ctx, imageId); err != nil {
		return false, err
	} else if exist {
		return shootECSClient.CheckIfImageOwnedByAliCloud(imageId)
	}
	return false, nil
}

func (s *shootMutator) getImageId(ctx context.Context, imageName, imageRegion, cloudProfileName string, imageVersion, architecture *string) (string, error) {
	var (
		cloudProfile    = &corev1beta1.CloudProfile{}
		cloudProfileKey = kutil.Key(cloudProfileName)
	)
	if err := kutil.LookupObject(ctx, s.client, s.apiReader, cloudProfileKey, cloudProfile); err != nil {
		return "", err
	}
	cloudProfileConfig, err := s.getCloudProfileConfig(cloudProfile)
	if err != nil {
		return "", err
	}
	return helper.FindImageForRegionFromCloudProfile(cloudProfileConfig, imageName, *imageVersion, imageRegion, architecture)
}

func (s *shootMutator) getCloudProfileConfig(cloudProfile *corev1beta1.CloudProfile) (*api.CloudProfileConfig, error) {
	var cloudProfileConfig = &api.CloudProfileConfig{}
	if _, _, err := s.codec.Decode(cloudProfile.Spec.ProviderConfig.Raw, nil, cloudProfileConfig); err != nil {
		return nil, fmt.Errorf("could not decode providerConfig of cloudProfile for '%s': %w", kutil.ObjectName(cloudProfile), err)
	}

	return cloudProfileConfig, nil
}

func (s *shootMutator) mutateShootUpdate(ctx context.Context, shoot, oldShoot *corev1beta1.Shoot) error {
	if !equality.Semantic.DeepEqual(shoot.Spec, oldShoot.Spec) {
		if err := s.mutateControlPlaneConfigForUpdate(shoot, oldShoot); err != nil {
			return err
		}

		if err := s.triggerInfraUpdateForNewEncryptedSystemDisk(ctx, shoot, oldShoot); err != nil {
			return err
		}
	}
	if !equality.Semantic.DeepEqual(shoot.Spec, oldShoot.Spec) {
		s.mutateForEncryptedSystemDiskChange(shoot, oldShoot)
	}
	return nil
}

func (s *shootMutator) triggerInfraUpdateForNewEncryptedSystemDisk(ctx context.Context, shoot, oldshoot *corev1beta1.Shoot) error {
	for _, worker := range shoot.Spec.Provider.Workers {
		oldWorker := getWorkerByName(oldshoot, worker.Name)
		if oldWorker == nil {
			logger.Info("Set default value of encrypted disk for newly added worker")
			if err := s.setDefaultForEncryptedDisk(ctx, shoot, &worker); err != nil {
				return err
			}
			continue
		}
		if worker.Volume != nil && worker.Volume.Encrypted == nil && oldWorker.Volume != nil && oldWorker.Volume.Encrypted != nil {
			logger.Info("Encrypted disk flag for system disk is not set, keep old value")
			worker.Volume.Encrypted = oldWorker.Volume.Encrypted
		}
		oldDataVolumes := oldWorker.DataVolumes
		for i := range worker.DataVolumes {
			dataVolume := &worker.DataVolumes[i]
			oldDataVolume := getVolumeByName(oldDataVolumes, dataVolume.Name)
			if oldDataVolume == nil {
				if dataVolume.Encrypted == nil {
					logger.Info("Set encrypted disk by default for newly added data disk")
					dataVolume.Encrypted = pointer.BoolPtr(true)
				}
				continue
			}
			if dataVolume.Encrypted == nil && oldDataVolume.Encrypted != nil {
				logger.Info("Encrypted disk flag for data disk is not set, keep old value")
				dataVolume.Encrypted = oldDataVolume.Encrypted

			}
		}

	}
	return nil
}

func getWorkerByName(shoot *corev1beta1.Shoot, workerName string) *corev1beta1.Worker {

	for _, worker := range shoot.Spec.Provider.Workers {
		if worker.Name == workerName {
			return &worker
		}
	}
	return nil
}

func getVolumeByName(dataVolumes []corev1beta1.DataVolume, volumeName string) *corev1beta1.DataVolume {
	if dataVolumes == nil {
		return nil
	}
	for _, volume := range dataVolumes {
		if volume.Name == volumeName {
			return &volume
		}
	}
	return nil
}

func (s *shootMutator) mutateForEncryptedSystemDiskChange(shoot, oldShoot *corev1beta1.Shoot) {
	if requireNewEncryptedImage(shoot.Spec.Provider.Workers, oldShoot.Spec.Provider.Workers) {
		logger.Info("Need to reconcile infra as new encrypted system disk found in workers", "name", shoot.Name, "namespace", shoot.Namespace)
		if shoot.Annotations == nil {
			shoot.Annotations = make(map[string]string)
		}

		controllerutils.AddTasks(shoot.Annotations, v1beta1constants.ShootTaskDeployInfrastructure)
	}
}

// Check encrypted flag in new workers' volumes. If it is changed to be true, check for old workers
// if there is already a volume is set to be encrypted and also the OS version is the same.
func requireNewEncryptedImage(newWorkers, oldWorkers []corev1beta1.Worker) bool {
	var imagesEncrypted []*corev1beta1.ShootMachineImage
	for _, w := range oldWorkers {
		if w.Volume != nil && w.Volume.Encrypted != nil && *w.Volume.Encrypted {
			if w.Machine.Image != nil {
				imagesEncrypted = append(imagesEncrypted, w.Machine.Image)
			}
		}
	}

	for _, w := range newWorkers {
		if w.Volume != nil && w.Volume.Encrypted != nil && *w.Volume.Encrypted {
			if w.Machine.Image != nil {
				found := false
				for _, image := range imagesEncrypted {
					if w.Machine.Image.Name == image.Name && reflect.DeepEqual(w.Machine.Image.Version, image.Version) {
						found = true
						break
					}
				}

				if !found {
					return true
				}
			}
		}
	}

	return false
}

func (s *shootMutator) decodeControlPlaneConfig(provider *corev1beta1.Provider) (*apisalicloudv1alpha1.ControlPlaneConfig, error) {
	cpConfig := &apisalicloudv1alpha1.ControlPlaneConfig{}

	if provider.ControlPlaneConfig != nil {
		if _, _, err := s.codec.Decode(provider.ControlPlaneConfig.Raw, nil, cpConfig); err != nil {
			return nil, fmt.Errorf("could not decode providerConfig of controlplane: %w", err)
		}
	}

	return cpConfig, nil
}

func (s *shootMutator) convertToRawExtension(obj runtime.Object) (*runtime.RawExtension, error) {
	if obj == nil {
		return nil, nil
	}

	data, err := runtime.Encode(s.codec, obj)
	if err != nil {
		return nil, err
	}

	return &runtime.RawExtension{
		Raw: data,
	}, nil

}

func (s *shootMutator) mutateControlPlaneConfigForCreate(shoot *corev1beta1.Shoot) error {
	cpConfig, err := s.decodeControlPlaneConfig(&shoot.Spec.Provider)
	if err != nil {
		return err
	}

	if cpConfig.CSI == nil {
		cpConfig.CSI = &apisalicloudv1alpha1.CSI{
			EnableADController: pointer.BoolPtr(true),
		}
	} else {
		if cpConfig.CSI.EnableADController == nil {
			cpConfig.CSI.EnableADController = pointer.BoolPtr(true)
		}
	}

	raw, err := s.convertToRawExtension(cpConfig)
	if err != nil {
		return err
	}

	shoot.Spec.Provider.ControlPlaneConfig = raw

	return nil
}

func (s *shootMutator) mutateControlPlaneConfigForUpdate(newShoot, oldShoot *corev1beta1.Shoot) error {
	oldCPConfig, err := s.decodeControlPlaneConfig(&oldShoot.Spec.Provider)
	if err != nil {
		return err
	}

	newCPConfig, err := s.decodeControlPlaneConfig(&newShoot.Spec.Provider)
	if err != nil {
		return err
	}

	changed := false
	// If EnableADController in new shoot is nil, keep the old value
	if oldCPConfig.CSI != nil {
		if newCPConfig.CSI == nil {
			newCPConfig.CSI = &apisalicloudv1alpha1.CSI{EnableADController: oldCPConfig.CSI.EnableADController}
			changed = true
		} else if newCPConfig.CSI.EnableADController == nil {
			newCPConfig.CSI.EnableADController = oldCPConfig.CSI.EnableADController
			changed = true
		}
	}

	if changed {
		raw, err := s.convertToRawExtension(newCPConfig)
		if err != nil {
			return err
		}
		newShoot.Spec.Provider.ControlPlaneConfig = raw
	}

	return nil
}
