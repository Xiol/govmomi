/*
Copyright (c) 2024-2024 VMware, Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package simulator

import (
	"slices"

	"github.com/google/uuid"

	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

const (
	nativeKeyProvider = string(types.KmipClusterInfoKmsManagementTypeNativeProvider)
)

type CryptoManagerKmip struct {
	mo.CryptoManagerKmip

	keyIDToProviderID map[string]string
}

func (m *CryptoManagerKmip) init(r *Registry) {
	if m.keyIDToProviderID == nil {
		m.keyIDToProviderID = map[string]string{}
	}
}

func (m *CryptoManagerKmip) ListKmipServers(
	ctx *Context, req *types.ListKmipServers) soap.HasFault {

	body := methods.ListKmipServersBody{
		Res: &types.ListKmipServersResponse{},
	}

	if len(m.KmipServers) > 0 {
		limit := len(m.KmipServers)
		if req.Limit != nil {
			if reqLimit := int(*req.Limit); reqLimit >= 0 && reqLimit < limit {
				limit = reqLimit
			}
		}
		body.Res.Returnval = m.KmipServers[0:limit]
	}

	return &body

}

// TODO: Implement req.DefaultsToParent
func (m *CryptoManagerKmip) GetDefaultKmsCluster(
	ctx *Context, req *types.GetDefaultKmsCluster) soap.HasFault {

	var (
		providerID string
		body       methods.GetDefaultKmsClusterBody
	)

	for i := range m.KmipServers {
		c := m.KmipServers[i]
		if req.Entity != nil {
			for j := range c.UseAsEntityDefault {
				if *req.Entity == c.UseAsEntityDefault[j] {
					providerID = c.ClusterId.Id
				}
			}
		} else if c.UseAsDefault {
			providerID = c.ClusterId.Id
		}
		if providerID != "" {
			break
		}
	}

	if providerID == "" {
		body.Fault_ = Fault("No default provider", &types.RuntimeFault{})
	} else {
		body.Res = &types.GetDefaultKmsClusterResponse{
			Returnval: &types.KeyProviderId{Id: providerID},
		}
	}

	return &body
}

type retrieveKmipServerStatusTask struct {
	*CryptoManagerKmip
	get []types.KmipClusterInfo
	ctx *Context
}

func (c *retrieveKmipServerStatusTask) Run(
	task *Task) (types.AnyType, types.BaseMethodFault) {

	var result []types.CryptoManagerKmipClusterStatus

	if len(c.get) == 0 {
		c.get = make([]types.KmipClusterInfo, len(c.KmipServers))
		copy(c.get, c.KmipServers)
	}

	for i := range c.get {
		g := &c.get[i]
		if len(g.Servers) == 0 {
			for j := range c.KmipServers {
				if g.ClusterId.Id == c.KmipServers[j].ClusterId.Id {
					g.Servers = make(
						[]types.KmipServerInfo, len(c.KmipServers[j].Servers))
					copy(g.Servers, c.KmipServers[j].Servers)
				}
			}
		}
	}

	for i := range c.KmipServers {
		for j := range c.get {
			if c.KmipServers[i].ClusterId.Id == c.get[j].ClusterId.Id {
				clusterStatus := types.CryptoManagerKmipClusterStatus{
					ClusterId: types.KeyProviderId{
						Id: c.KmipServers[i].ClusterId.Id,
					},
					ManagementType: c.KmipServers[i].ManagementType,
					OverallStatus:  types.ManagedEntityStatusGreen,
				}
				for k := range c.KmipServers[i].Servers {
					for l := range c.get[j].Servers {
						if c.KmipServers[i].Servers[k].Name == c.get[j].Servers[l].Name {
							clusterStatus.Servers = append(
								clusterStatus.Servers,
								types.CryptoManagerKmipServerStatus{
									Name:   c.KmipServers[i].Servers[k].Name,
									Status: types.ManagedEntityStatusGreen,
								},
							)
						}
					}
				}
				result = append(result, clusterStatus)
			}
		}
	}

	return types.ArrayOfCryptoManagerKmipClusterStatus{
		CryptoManagerKmipClusterStatus: result,
	}, nil
}

func (m *CryptoManagerKmip) RetrieveKmipServersStatusTask(
	ctx *Context, req *types.RetrieveKmipServersStatus_Task) soap.HasFault {

	var body methods.RetrieveKmipServersStatus_TaskBody

	runner := &retrieveKmipServerStatusTask{
		CryptoManagerKmip: m,
		ctx:               ctx,
		get:               req.Clusters,
	}
	task := CreateTask(
		runner.Reference(),
		"retrieveKmipServerStatus",
		runner.Run)

	body.Res = &types.RetrieveKmipServersStatus_TaskResponse{
		Returnval: task.Run(ctx),
	}

	return &body
}

func (m *CryptoManagerKmip) MarkDefault(
	ctx *Context, req *types.MarkDefault) soap.HasFault {

	return m.SetDefaultKmsCluster(
		ctx,
		&types.SetDefaultKmsCluster{
			This: req.This,
			ClusterId: &types.KeyProviderId{
				Id: req.ClusterId.Id,
			},
		})

}

func (m *CryptoManagerKmip) SetDefaultKmsCluster(
	ctx *Context, req *types.SetDefaultKmsCluster) soap.HasFault {

	var (
		validClusterID bool
		body           methods.SetDefaultKmsClusterBody
	)

	for i := range m.KmipServers {
		c := &m.KmipServers[i]
		if req.ClusterId != nil && req.ClusterId.Id != "" {
			if c.ClusterId.Id != req.ClusterId.Id {
				c.UseAsDefault = false
				c.UseAsEntityDefault = nil
			} else {
				validClusterID = true
				if req.Entity == nil {
					c.UseAsDefault = true
				} else {
					found := false
					for j := range c.UseAsEntityDefault {
						if *req.Entity == c.UseAsEntityDefault[j] {
							found = true
							break
						}
					}
					if !found {
						c.UseAsEntityDefault = append(
							c.UseAsEntityDefault,
							*req.Entity)
					}
				}
			}
		} else if req.Entity != nil {
			x := -1
			for j := range c.UseAsEntityDefault {
				if *req.Entity == c.UseAsEntityDefault[j] {
					x = j
					break
				}
			}
			if x >= 0 {
				c.UseAsEntityDefault = slices.Delete(
					c.UseAsEntityDefault, x, x+1)
			}
		} else {
			c.UseAsDefault = false
		}
	}

	if req.ClusterId != nil && req.ClusterId.Id != "" && !validClusterID {
		body.Fault_ = Fault("Invalid cluster ID", &types.RuntimeFault{})
	} else {
		body.Res = &types.SetDefaultKmsClusterResponse{}
	}

	return &body
}

func (m *CryptoManagerKmip) RegisterKmsCluster(
	ctx *Context, req *types.RegisterKmsCluster) soap.HasFault {

	var body methods.RegisterKmsClusterBody

	for i := range m.KmipServers {
		if req.ClusterId.Id == m.KmipServers[i].ClusterId.Id {
			body.Fault_ = Fault("Already registered", &types.RuntimeFault{})
		}
	}
	if body.Fault_ == nil {
		body.Res = &types.RegisterKmsClusterResponse{}
		m.KmipServers = append(m.KmipServers,
			types.KmipClusterInfo{
				ClusterId: types.KeyProviderId{
					Id: req.ClusterId.Id,
				},
				ManagementType: req.ManagementType,
			})
	}

	return &body
}

func (m *CryptoManagerKmip) UnregisterKmsCluster(
	ctx *Context, req *types.UnregisterKmsCluster) soap.HasFault {

	var body methods.UnregisterKmsClusterBody

	x := -1
	for i := range m.KmipServers {
		if req.ClusterId.Id == m.KmipServers[i].ClusterId.Id {
			x = i
		}
	}

	if x < 0 {
		body.Fault_ = Fault("Invalid cluster ID", &types.RuntimeFault{})
	} else {
		m.KmipServers = slices.Delete(m.KmipServers, x, x+1)
		body.Res = &types.UnregisterKmsClusterResponse{}
	}

	return &body
}

func (m *CryptoManagerKmip) RegisterKmipServer(
	ctx *Context, req *types.RegisterKmipServer) soap.HasFault {

	var (
		validClusterID    bool
		alreadyRegistered bool
		body              methods.RegisterKmipServerBody
	)

	for i := range m.KmipServers {
		c := &m.KmipServers[i]

		if req.Server.ClusterId.Id == c.ClusterId.Id {
			validClusterID = true
			for j := range c.Servers {
				if req.Server.Info.Name == c.Servers[j].Name {
					alreadyRegistered = true
					break
				}
			}
			if !alreadyRegistered {
				c.Servers = append(c.Servers, req.Server.Info)
			}
		}

		if validClusterID || alreadyRegistered {
			break
		}
	}

	if !validClusterID {
		body.Fault_ = Fault("Invalid cluster ID", &types.RuntimeFault{})
	} else if alreadyRegistered {
		body.Fault_ = Fault("Already registered", &types.RuntimeFault{})
	} else {
		body.Res = &types.RegisterKmipServerResponse{}
	}

	return &body
}

func (m *CryptoManagerKmip) RemoveKmipServer(
	ctx *Context, req *types.RemoveKmipServer) soap.HasFault {

	var (
		validClusterID  bool
		validServerName bool
		body            methods.RemoveKmipServerBody
	)

	for i := range m.KmipServers {
		c := &m.KmipServers[i]

		if req.ClusterId.Id == c.ClusterId.Id {
			validClusterID = true

			x := -1
			for j := range c.Servers {
				if req.ServerName == c.Servers[j].Name {
					x = j
					break
				}
			}

			if x >= 0 {
				validServerName = true
				c.Servers = slices.Delete(c.Servers, x, x+1)
			}
		}

		if validClusterID {
			break
		}
	}

	if !validClusterID {
		body.Fault_ = Fault("Invalid cluster ID", &types.RuntimeFault{})
	} else if !validServerName {
		body.Fault_ = Fault("Invalid server name", &types.RuntimeFault{})
	} else {
		body.Res = &types.RemoveKmipServerResponse{}
	}

	return &body
}

func (m *CryptoManagerKmip) UpdateKmipServer(
	ctx *Context, req *types.UpdateKmipServer) soap.HasFault {

	var (
		validClusterID  bool
		validServerName bool
		body            methods.UpdateKmipServerBody
	)

	for i := range m.KmipServers {
		c := &m.KmipServers[i]

		if req.Server.ClusterId.Id == c.ClusterId.Id {
			validClusterID = true
			for j := range c.Servers {
				if req.Server.Info.Name == c.Servers[j].Name {
					validServerName = true
					c.Servers[j] = req.Server.Info
					break
				}
			}
		}

		if validClusterID {
			break
		}
	}

	if !validClusterID {
		body.Fault_ = Fault("Invalid cluster ID", &types.RuntimeFault{})
	} else if !validServerName {
		body.Fault_ = Fault("Invalid server name", &types.RuntimeFault{})
	} else {
		body.Res = &types.UpdateKmipServerResponse{}
	}

	return &body
}

func (m *CryptoManagerKmip) GenerateKey(
	ctx *Context, req *types.GenerateKey) soap.HasFault {

	var (
		provider types.KmipClusterInfo
		body     methods.GenerateKeyBody
	)

	for i := range m.KmipServers {
		c := m.KmipServers[i]
		if req.KeyProvider == nil {
			if c.UseAsDefault {
				provider = c
			}
		} else if req.KeyProvider.Id == c.ClusterId.Id {
			provider = c
		}
		if provider.ClusterId.Id != "" {
			break
		}
	}

	if provider.ClusterId.Id == "" {
		body.Fault_ = Fault("No default provider", &types.RuntimeFault{})
	} else if provider.ManagementType == nativeKeyProvider {
		body.Fault_ = Fault(
			"Cannot generate keys with native key provider",
			&types.RuntimeFault{})
	} else {
		newKey := uuid.NewString()
		m.keyIDToProviderID[newKey] = provider.ClusterId.Id

		body.Res = &types.GenerateKeyResponse{
			Returnval: types.CryptoKeyResult{
				Success: true,
				KeyId: types.CryptoKeyId{
					KeyId: newKey,
					ProviderId: &types.KeyProviderId{
						Id: provider.ClusterId.Id,
					},
				},
			},
		}
	}

	return &body
}

func (m *CryptoManagerKmip) ListKeys(
	ctx *Context, req *types.ListKeys) soap.HasFault {

	body := methods.ListKeysBody{
		Res: &types.ListKeysResponse{},
	}

	if len(m.keyIDToProviderID) > 0 {
		var (
			i     int
			limit = len(m.keyIDToProviderID)
		)
		if req.Limit != nil {
			if reqLimit := int(*req.Limit); reqLimit >= 0 && reqLimit < limit {
				limit = reqLimit
			}
		}
		for keyID, providerID := range m.keyIDToProviderID {
			if i >= limit {
				break
			}
			i++
			body.Res.Returnval = append(body.Res.Returnval, types.CryptoKeyId{
				KeyId: keyID,
				ProviderId: &types.KeyProviderId{
					Id: providerID,
				},
			})
		}
	}

	return &body
}
