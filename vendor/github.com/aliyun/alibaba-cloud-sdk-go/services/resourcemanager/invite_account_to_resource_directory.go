package resourcemanager

//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.
//
// Code generated by Alibaba Cloud SDK Code Generator.
// Changes may cause incorrect behavior and will be lost if the code is regenerated.

import (
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
)

// InviteAccountToResourceDirectory invokes the resourcemanager.InviteAccountToResourceDirectory API synchronously
// api document: https://help.aliyun.com/api/resourcemanager/inviteaccounttoresourcedirectory.html
func (client *Client) InviteAccountToResourceDirectory(request *InviteAccountToResourceDirectoryRequest) (response *InviteAccountToResourceDirectoryResponse, err error) {
	response = CreateInviteAccountToResourceDirectoryResponse()
	err = client.DoAction(request, response)
	return
}

// InviteAccountToResourceDirectoryWithChan invokes the resourcemanager.InviteAccountToResourceDirectory API asynchronously
// api document: https://help.aliyun.com/api/resourcemanager/inviteaccounttoresourcedirectory.html
// asynchronous document: https://help.aliyun.com/document_detail/66220.html
func (client *Client) InviteAccountToResourceDirectoryWithChan(request *InviteAccountToResourceDirectoryRequest) (<-chan *InviteAccountToResourceDirectoryResponse, <-chan error) {
	responseChan := make(chan *InviteAccountToResourceDirectoryResponse, 1)
	errChan := make(chan error, 1)
	err := client.AddAsyncTask(func() {
		defer close(responseChan)
		defer close(errChan)
		response, err := client.InviteAccountToResourceDirectory(request)
		if err != nil {
			errChan <- err
		} else {
			responseChan <- response
		}
	})
	if err != nil {
		errChan <- err
		close(responseChan)
		close(errChan)
	}
	return responseChan, errChan
}

// InviteAccountToResourceDirectoryWithCallback invokes the resourcemanager.InviteAccountToResourceDirectory API asynchronously
// api document: https://help.aliyun.com/api/resourcemanager/inviteaccounttoresourcedirectory.html
// asynchronous document: https://help.aliyun.com/document_detail/66220.html
func (client *Client) InviteAccountToResourceDirectoryWithCallback(request *InviteAccountToResourceDirectoryRequest, callback func(response *InviteAccountToResourceDirectoryResponse, err error)) <-chan int {
	result := make(chan int, 1)
	err := client.AddAsyncTask(func() {
		var response *InviteAccountToResourceDirectoryResponse
		var err error
		defer close(result)
		response, err = client.InviteAccountToResourceDirectory(request)
		callback(response, err)
		result <- 1
	})
	if err != nil {
		defer close(result)
		callback(nil, err)
		result <- 0
	}
	return result
}

// InviteAccountToResourceDirectoryRequest is the request struct for api InviteAccountToResourceDirectory
type InviteAccountToResourceDirectoryRequest struct {
	*requests.RpcRequest
	Note         string `position:"Query" name:"Note"`
	TargetType   string `position:"Query" name:"TargetType"`
	TargetEntity string `position:"Query" name:"TargetEntity"`
}

// InviteAccountToResourceDirectoryResponse is the response struct for api InviteAccountToResourceDirectory
type InviteAccountToResourceDirectoryResponse struct {
	*responses.BaseResponse
	RequestId string    `json:"RequestId" xml:"RequestId"`
	Handshake Handshake `json:"Handshake" xml:"Handshake"`
}

// CreateInviteAccountToResourceDirectoryRequest creates a request to invoke InviteAccountToResourceDirectory API
func CreateInviteAccountToResourceDirectoryRequest() (request *InviteAccountToResourceDirectoryRequest) {
	request = &InviteAccountToResourceDirectoryRequest{
		RpcRequest: &requests.RpcRequest{},
	}
	request.InitWithApiInfo("ResourceManager", "2020-03-31", "InviteAccountToResourceDirectory", "resourcemanager", "openAPI")
	return
}

// CreateInviteAccountToResourceDirectoryResponse creates a response to parse from InviteAccountToResourceDirectory response
func CreateInviteAccountToResourceDirectoryResponse() (response *InviteAccountToResourceDirectoryResponse) {
	response = &InviteAccountToResourceDirectoryResponse{
		BaseResponse: &responses.BaseResponse{},
	}
	return
}
