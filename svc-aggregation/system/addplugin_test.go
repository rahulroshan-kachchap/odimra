//(C) Copyright [2020] Hewlett Packard Enterprise Development LP
//
//Licensed under the Apache License, Version 2.0 (the "License"); you may
//not use this file except in compliance with the License. You may obtain
//a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//License for the specific language governing permissions and limitations
// under the License.

package system

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/ODIM-Project/ODIM/lib-utilities/common"
	"github.com/ODIM-Project/ODIM/lib-utilities/config"
	aggregatorproto "github.com/ODIM-Project/ODIM/lib-utilities/proto/aggregator"
	"github.com/ODIM-Project/ODIM/lib-utilities/response"
	"github.com/ODIM-Project/ODIM/svc-aggregation/agmodel"
)

func mockData(t *testing.T, dbType common.DbType, table, id string, data interface{}) {
	connPool, err := common.GetDBConnection(dbType)
	if err != nil {
		t.Fatalf("error: mockData() failed to DB connection: %v", err)
	}
	if err = connPool.Create(table, id, data); err != nil {
		t.Fatalf("error: mockData() failed to create entry %s-%s: %v", table, id, err)
	}
}

func TestExternalInterface_Plugin(t *testing.T) {
	config.SetUpMockConfig(t)
	addComputeRetrieval := config.AddComputeSkipResources{
		SystemCollection: []string{"Chassis", "LogServices"},
	}
	err := mockPluginData(t, "ILO")
	if err != nil {
		t.Fatalf("Error in creating mock PluginData :%v", err)
	}

	// create plugin with bad password for decryption failure
	pluginData := agmodel.Plugin{
		Password: []byte("password"),
		ID:       "PluginWithBadPassword",
	}
	mockData(t, common.OnDisk, "Plugin", "PluginWithBadPassword", pluginData)
	// create plugin with bad data
	mockData(t, common.OnDisk, "Plugin", "PluginWithBadData", "PluginWithBadData")

	config.Data.AddComputeSkipResources = &addComputeRetrieval
	defer func() {
		err := common.TruncateDB(common.OnDisk)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		err = common.TruncateDB(common.InMemory)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
	}()
	reqSuccess, _ := json.Marshal(AddResourceRequest{
		ManagerAddress: "localhost:9091",
		UserName:       "admin",
		Password:       "password",
		Oem: &AddOEM{
			PluginID:          "GRF",
			PreferredAuthType: "BasicAuth",
			PluginType:        "Compute",
		},
	})
	reqExistingPlugin, _ := json.Marshal(AddResourceRequest{
		ManagerAddress: "localhost:9091",
		UserName:       "admin",
		Password:       "password",
		Oem: &AddOEM{
			PluginID:          "ILO",
			PreferredAuthType: "BasicAuth",
			PluginType:        "Compute",
		},
	})
	reqInvalidAuthType, _ := json.Marshal(AddResourceRequest{
		ManagerAddress: "localhost:9091",
		UserName:       "admin",
		Password:       "password",
		Oem: &AddOEM{
			PluginID:          "ILO",
			PreferredAuthType: "BasicAuthentication",
			PluginType:        "Compute",
		},
	})
	reqInvalidPluginType, _ := json.Marshal(AddResourceRequest{
		ManagerAddress: "localhost:9091",
		UserName:       "admin",
		Password:       "password",
		Oem: &AddOEM{
			PluginID:          "ILO",
			PreferredAuthType: "BasicAuth",
			PluginType:        "plugin",
		},
	})
	reqExistingPluginBadPassword, _ := json.Marshal(AddResourceRequest{
		ManagerAddress: "localhost:9091",
		UserName:       "admin",
		Password:       "password",
		Oem: &AddOEM{
			PluginID:          "PluginWithBadPassword",
			PreferredAuthType: "BasicAuth",
			PluginType:        "Compute",
		},
	})
	reqExistingPluginBadData, _ := json.Marshal(AddResourceRequest{
		ManagerAddress: "localhost:9091",
		UserName:       "admin",
		Password:       "password",
		Oem: &AddOEM{
			PluginID:          "PluginWithBadData",
			PreferredAuthType: "BasicAuth",
			PluginType:        "Compute",
		},
	})

	p := &ExternalInterface{
		ContactClient:     mockContactClient,
		Auth:              mockIsAuthorized,
		CreateChildTask:   mockCreateChildTask,
		UpdateTask:        mockUpdateTask,
		CreateSubcription: EventFunctionsForTesting,
		PublishEvent:      PostEventFunctionForTesting,
		GetPluginStatus:   GetPluginStatusForTesting,
		SubscribeToEMB:    mockSubscribeEMB,
		EncryptPassword:   stubDevicePassword,
		DecryptPassword:   stubDevicePassword,
	}

	type args struct {
		taskID string
		req    *aggregatorproto.AggregatorRequest
	}
	tests := []struct {
		name string
		p    *ExternalInterface
		args args
		want response.RPC
	}{
		{
			name: "posivite case",
			p:    p,
			args: args{
				taskID: "123",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  reqSuccess,
				},
			},
			want: response.RPC{
				StatusCode: http.StatusOK,
			},
		},
		{
			name: "Existing Plugin",
			p:    p,
			args: args{
				taskID: "123",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  reqExistingPlugin,
				},
			},
			want: response.RPC{
				StatusCode: http.StatusConflict,
			},
		}, {
			name: "Invalid Auth type",
			p:    p,
			args: args{
				taskID: "123",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  reqInvalidAuthType,
				},
			},
			want: response.RPC{
				StatusCode: http.StatusBadRequest,
			},
		}, {
			name: "Invalid Plugin type",
			p:    p,
			args: args{
				taskID: "123",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  reqInvalidPluginType,
				},
			},
			want: response.RPC{
				StatusCode: http.StatusBadRequest,
			},
		}, {
			name: "Existing Plugin with bad password",
			p:    p,
			args: args{
				taskID: "123",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  reqExistingPluginBadPassword,
				},
			},
			want: response.RPC{
				StatusCode: http.StatusConflict,
			},
		}, {
			name: "Existing Plugin with bad data",
			p:    p,
			args: args{
				taskID: "123",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  reqExistingPluginBadData,
				},
			},
			want: response.RPC{
				StatusCode: http.StatusConflict,
			},
		},
	}
	for _, tt := range tests {
		ActiveReqSet.ReqRecord = make(map[string]interface{})
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.AggregationServiceAdd(tt.args.taskID, "validUserName", tt.args.req); !reflect.DeepEqual(got.StatusCode, tt.want.StatusCode) {
				t.Errorf("ExternalInterface.AggregationServiceAdd = %v, want %v", got, tt.want)
			}
		})
		ActiveReqSet.ReqRecord = nil
	}
}

func TestExternalInterface_PluginXAuth(t *testing.T) {
	config.SetUpMockConfig(t)
	addComputeRetrieval := config.AddComputeSkipResources{
		SystemCollection: []string{"Chassis", "LogServices"},
	}
	err := mockPluginData(t, "XAuthPlugin")
	if err != nil {
		t.Fatalf("Error in creating mock PluginData :%v", err)
	}

	config.Data.AddComputeSkipResources = &addComputeRetrieval
	defer func() {
		err := common.TruncateDB(common.OnDisk)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		err = common.TruncateDB(common.InMemory)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
	}()

	if err != nil {
		t.Fatalf("error while trying to create schema: %v", err)
	}
	reqXAuthSuccess, _ := json.Marshal(AddResourceRequest{
		ManagerAddress: "localhost:9091",
		UserName:       "admin",
		Password:       "password",
		Oem: &AddOEM{
			PluginID:          "GRF",
			PreferredAuthType: "XAuthToken",
			PluginType:        "Compute",
		},
	})
	reqXAuthFail, _ := json.Marshal(AddResourceRequest{
		ManagerAddress: "localhost:9091",
		UserName:       "incorrectusername",
		Password:       "incorrectPassword",
		Oem: &AddOEM{
			PluginID:          "ILO",
			PreferredAuthType: "XAuthToken",
			PluginType:        "Compute",
		},
	})

	reqStatusFail, _ := json.Marshal(AddResourceRequest{
		ManagerAddress: "100.0.0.3:9091",
		UserName:       "admin",
		Password:       "password",
		Oem: &AddOEM{
			PluginID:          "ILO",
			PreferredAuthType: "XAuthToken",
			PluginType:        "Compute",
		},
	})

	reqInvalidStatusBody, _ := json.Marshal(AddResourceRequest{
		ManagerAddress: "100.0.0.4:9091",
		UserName:       "admin",
		Password:       "password",
		Oem: &AddOEM{
			PluginID:          "ILO",
			PreferredAuthType: "XAuthToken",
			PluginType:        "Compute",
		},
	})

	reqManagerGetFail, _ := json.Marshal(AddResourceRequest{
		ManagerAddress: "100.0.0.5:9091",
		UserName:       "admin",
		Password:       "password",
		Oem: &AddOEM{
			PluginID:          "ILO",
			PreferredAuthType: "XAuthToken",
			PluginType:        "Compute",
		},
	})

	reqInvalidManagerBody, _ := json.Marshal(AddResourceRequest{
		ManagerAddress: "100.0.0.6:9091",
		UserName:       "admin",
		Password:       "password",
		Oem: &AddOEM{
			PluginID:          "ILO",
			PreferredAuthType: "XAuthToken",
			PluginType:        "Compute",
		},
	})

	p := &ExternalInterface{
		ContactClient:     mockContactClient,
		Auth:              mockIsAuthorized,
		CreateChildTask:   mockCreateChildTask,
		UpdateTask:        mockUpdateTask,
		CreateSubcription: EventFunctionsForTesting,
		PublishEvent:      PostEventFunctionForTesting,
		GetPluginStatus:   GetPluginStatusForTesting,
		SubscribeToEMB:    mockSubscribeEMB,
		EncryptPassword:   stubDevicePassword,
		DecryptPassword:   stubDevicePassword,
	}

	type args struct {
		taskID string
		req    *aggregatorproto.AggregatorRequest
	}
	tests := []struct {
		name string
		p    *ExternalInterface
		args args
		want response.RPC
	}{
		{
			name: "posivite case with XAuthToken",
			p:    p,
			args: args{
				taskID: "123",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  reqXAuthSuccess,
				},
			},
			want: response.RPC{
				StatusCode: http.StatusOK,
			},
		},
		{
			name: "Failure with XAuthToken",
			p:    p,
			args: args{
				taskID: "123",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  reqXAuthFail,
				},
			},
			want: response.RPC{
				StatusCode: http.StatusUnauthorized,
			},
		},
		{
			name: "Failure with Status Check",
			p:    p,
			args: args{
				taskID: "123",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  reqStatusFail,
				},
			},
			want: response.RPC{
				StatusCode: http.StatusServiceUnavailable,
			},
		},
		{
			name: "incorrect status body",
			p:    p,
			args: args{
				taskID: "123",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  reqInvalidStatusBody,
				},
			},
			want: response.RPC{
				StatusCode: http.StatusInternalServerError,
			},
		},
		{
			name: "Failure with Manager Get",
			p:    p,
			args: args{
				taskID: "123",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  reqManagerGetFail,
				},
			},
			want: response.RPC{
				StatusCode: http.StatusServiceUnavailable,
			},
		},
		{
			name: "incorrect manager body",
			p:    p,
			args: args{
				taskID: "123",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  reqInvalidManagerBody,
				},
			},
			want: response.RPC{
				StatusCode: http.StatusInternalServerError,
			},
		},
	}
	for _, tt := range tests {
		ActiveReqSet.ReqRecord = make(map[string]interface{})
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.AggregationServiceAdd(tt.args.taskID, "validUserName", tt.args.req); !reflect.DeepEqual(got.StatusCode, tt.want.StatusCode) {
				t.Errorf("ExternalInterface.AggregationServiceAdd = %v, want %v", got, tt.want)
			}
		})
		ActiveReqSet.ReqRecord = nil
	}
}

func TestExternalInterface_PluginWithMultipleRequest(t *testing.T) {
	config.SetUpMockConfig(t)
	addComputeRetrieval := config.AddComputeSkipResources{
		SystemCollection: []string{"Chassis", "LogServices"},
	}

	config.Data.AddComputeSkipResources = &addComputeRetrieval
	defer func() {
		common.TruncateDB(common.OnDisk)
		common.TruncateDB(common.InMemory)
	}()

	reqSuccess, _ := json.Marshal(AddResourceRequest{
		ManagerAddress: "localhost:9091",
		UserName:       "admin",
		Password:       "password",
		Oem: &AddOEM{
			PluginID:          "GRF",
			PreferredAuthType: "BasicAuth",
			PluginType:        "Compute",
		},
	})

	p := &ExternalInterface{
		ContactClient:     testContactClientWithDelay,
		Auth:              mockIsAuthorized,
		CreateChildTask:   mockCreateChildTask,
		UpdateTask:        mockUpdateTask,
		CreateSubcription: EventFunctionsForTesting,
		PublishEvent:      PostEventFunctionForTesting,
		GetPluginStatus:   GetPluginStatusForTesting,
		SubscribeToEMB:    mockSubscribeEMB,
		EncryptPassword:   stubDevicePassword,
		DecryptPassword:   stubDevicePassword,
	}

	type args struct {
		taskID string
		req    *aggregatorproto.AggregatorRequest
	}
	req := &aggregatorproto.AggregatorRequest{
		SessionToken: "validToken",
		RequestBody:  reqSuccess,
	}
	tests := []struct {
		name string
		p    *ExternalInterface
		args args
		want response.RPC
	}{
		{
			name: "multiple request",
			want: response.RPC{
				StatusCode: http.StatusConflict,
			},
		},
	}
	for _, tt := range tests {
		ActiveReqSet.ReqRecord = make(map[string]interface{})
		t.Run(tt.name, func(t *testing.T) {
			go p.AggregationServiceAdd("123", "validUserName", req)
			time.Sleep(time.Second)
			if got := p.AggregationServiceAdd("123", "validUserName", req); !reflect.DeepEqual(got.StatusCode, tt.want.StatusCode) {
				t.Errorf("ExternalInterface.AggregationServiceAdd = %v, want %v", got, tt.want)
			}
		})
		ActiveReqSet.ReqRecord = nil
	}
}
