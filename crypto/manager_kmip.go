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

package crypto

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/types"
)

type ManagerKmip struct {
	object.Common
}

// GetManagerKmip wraps NewManager, returning ErrNotSupported when the client is
// not connected to a vCenter instance.
func GetManagerKmip(c *vim25.Client) (*ManagerKmip, error) {
	if c.ServiceContent.CryptoManager == nil {
		return nil, object.ErrNotSupported
	}
	return NewManagerKmip(c), nil
}

func NewManagerKmip(c *vim25.Client) *ManagerKmip {
	m := ManagerKmip{
		Common: object.NewCommon(c, *c.ServiceContent.CryptoManager),
	}
	return &m
}

func (m ManagerKmip) ListKmipServers(
	ctx context.Context,
	limit *int32) ([]types.KmipClusterInfo, error) {

	req := types.ListKmipServers{
		This:  m.Reference(),
		Limit: limit,
	}
	res, err := methods.ListKmipServers(ctx, m.Client(), &req)
	if err != nil {
		return nil, err
	}
	return res.Returnval, nil
}

func (m ManagerKmip) IsDefaultProviderNative(
	ctx context.Context,
	entity *types.ManagedObjectReference,
	defaultsToParent bool) (bool, error) {

	defaultProviderID, err := m.GetDefaultKmsClusterID(
		ctx, entity, defaultsToParent)
	if err != nil {
		return false, err
	}
	if defaultProviderID == "" {
		return false, nil
	}
	return m.IsNativeProvider(ctx, defaultProviderID)
}

func (m ManagerKmip) IsNativeProvider(
	ctx context.Context,
	providerID string) (bool, error) {

	info, err := m.GetClusterStatus(ctx, providerID)
	if err != nil {
		return false, err
	}
	if info == nil {
		return false, nil
	}
	return info.ManagementType == string(
		types.KmipClusterInfoKmsManagementTypeNativeProvider), nil
}

func (m ManagerKmip) GetDefaultKmsClusterID(
	ctx context.Context,
	entity *types.ManagedObjectReference,
	defaultsToParent bool) (string, error) {

	req := types.GetDefaultKmsCluster{
		This:             m.Reference(),
		Entity:           entity,
		DefaultsToParent: &defaultsToParent,
	}
	res, err := methods.GetDefaultKmsCluster(ctx, m.Client(), &req)
	if err != nil {
		return "", err
	}
	if res.Returnval != nil {
		return res.Returnval.Id, nil
	}
	return "", nil
}

func (m ManagerKmip) GetStatus(
	ctx context.Context,
	clusters ...types.KmipClusterInfo) ([]types.CryptoManagerKmipClusterStatus, error) {

	req := types.RetrieveKmipServersStatus_Task{
		This:     m.Reference(),
		Clusters: clusters,
	}
	res, err := methods.RetrieveKmipServersStatus_Task(ctx, m.Client(), &req)
	if err != nil {
		return nil, err
	}

	task := object.NewTask(m.Client(), res.Returnval)
	taskInfo, err := task.WaitForResult(ctx)
	if err != nil {
		return nil, err
	}

	if taskInfo.Result == nil {
		return nil, nil
	}
	result, ok := taskInfo.Result.(types.ArrayOfCryptoManagerKmipClusterStatus)
	if !ok {
		return nil, nil
	}
	if len(result.CryptoManagerKmipClusterStatus) == 0 {
		return nil, nil
	}

	return result.CryptoManagerKmipClusterStatus, nil
}

func (m ManagerKmip) GetClusterStatus(
	ctx context.Context,
	providerID string) (*types.CryptoManagerKmipClusterStatus, error) {

	result, err := m.GetStatus(
		ctx,
		types.KmipClusterInfo{
			ClusterId: types.KeyProviderId{
				Id: providerID,
			},
		})
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("invalid cluster ID")
	}
	return &result[0], nil
}

func (m ManagerKmip) GetServerStatus(
	ctx context.Context,
	providerID, serverName string) (*types.CryptoManagerKmipServerStatus, error) {

	result, err := m.GetStatus(
		ctx,
		types.KmipClusterInfo{
			ClusterId: types.KeyProviderId{
				Id: providerID,
			},
			Servers: []types.KmipServerInfo{
				{
					Name: serverName,
				},
			},
		})
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("invalid cluster ID")
	}
	if len(result[0].Servers) == 0 {
		return nil, fmt.Errorf("invalid server name")
	}
	return &result[0].Servers[0], nil
}

func (m ManagerKmip) MarkDefault(
	ctx context.Context,
	providerID string) error {

	req := types.MarkDefault{
		This:      m.Reference(),
		ClusterId: types.KeyProviderId{Id: providerID},
	}
	_, err := methods.MarkDefault(ctx, m.Client(), &req)
	if err != nil {
		return err
	}
	return nil
}

func (m ManagerKmip) SetDefaultKmsClusterId(
	ctx context.Context,
	providerID string,
	entity *types.ManagedObjectReference) error {

	req := types.SetDefaultKmsCluster{
		This:   m.Reference(),
		Entity: entity,
	}
	if providerID != "" {
		req.ClusterId = &types.KeyProviderId{
			Id: providerID,
		}
	}
	_, err := methods.SetDefaultKmsCluster(ctx, m.Client(), &req)
	if err != nil {
		return err
	}
	return nil
}

func (m ManagerKmip) RegisterKmipCluster(
	ctx context.Context,
	providerID string,
	managementType types.KmipClusterInfoKmsManagementType) error {

	req := types.RegisterKmsCluster{
		This: m.Reference(),
		ClusterId: types.KeyProviderId{
			Id: providerID,
		},
		ManagementType: string(managementType),
	}
	_, err := methods.RegisterKmsCluster(ctx, m.Client(), &req)
	if err != nil {
		return err
	}
	return nil
}

func (m ManagerKmip) UnregisterKmsCluster(
	ctx context.Context,
	providerID string) error {

	req := types.UnregisterKmsCluster{
		This: m.Reference(),
		ClusterId: types.KeyProviderId{
			Id: providerID,
		},
	}
	_, err := methods.UnregisterKmsCluster(ctx, m.Client(), &req)
	if err != nil {
		return err
	}
	return nil
}

func (m ManagerKmip) RegisterKmipServer(
	ctx context.Context,
	server types.KmipServerSpec) error {

	req := types.RegisterKmipServer{
		This:   m.Reference(),
		Server: server,
	}
	_, err := methods.RegisterKmipServer(ctx, m.Client(), &req)
	if err != nil {
		return err
	}
	return nil
}

func (m ManagerKmip) UpdateKmipServer(
	ctx context.Context,
	server types.KmipServerSpec) error {

	req := types.UpdateKmipServer{
		This:   m.Reference(),
		Server: server,
	}
	_, err := methods.UpdateKmipServer(ctx, m.Client(), &req)
	if err != nil {
		return err
	}
	return nil
}

func (m ManagerKmip) RemoveKmipServer(
	ctx context.Context,
	providerID, serverName string) error {

	req := types.RemoveKmipServer{
		This: m.Reference(),
		ClusterId: types.KeyProviderId{
			Id: providerID,
		},
		ServerName: serverName,
	}
	_, err := methods.RemoveKmipServer(ctx, m.Client(), &req)
	if err != nil {
		return err
	}
	return nil
}

func (m ManagerKmip) ListKeys(
	ctx context.Context,
	limit *int32) ([]types.CryptoKeyId, error) {

	req := types.ListKeys{
		This:  m.Reference(),
		Limit: limit,
	}
	res, err := methods.ListKeys(ctx, m.Client(), &req)
	if err != nil {
		return nil, err
	}
	return res.Returnval, nil
}

func (m ManagerKmip) IsValidKey(
	ctx context.Context,
	keyID string) (bool, error) {

	keys, err := m.ListKeys(ctx, nil)
	if err != nil {
		return false, err
	}

	for i := range keys {
		if keys[i].KeyId == keyID {
			return true, nil
		}
	}

	return false, nil
}

func (m ManagerKmip) IsValidProvider(
	ctx context.Context,
	providerID string) (bool, error) {

	clusters, err := m.ListKmipServers(ctx, nil)
	if err != nil {
		return false, err
	}

	for i := range clusters {
		if clusters[i].ClusterId.Id == providerID {
			return true, nil
		}
	}

	return false, nil
}

func (m ManagerKmip) IsValidServer(
	ctx context.Context,
	providerID, serverName string) (bool, error) {

	clusters, err := m.ListKmipServers(ctx, nil)
	if err != nil {
		return false, err
	}

	for i := range clusters {
		if clusters[i].ClusterId.Id == providerID {
			for j := range clusters[i].Servers {
				if clusters[i].Servers[j].Name == serverName {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

func (m ManagerKmip) GenerateKey(
	ctx context.Context,
	providerID string) (string, error) {

	req := types.GenerateKey{
		This: m.Reference(),
	}

	if providerID != "" {
		req.KeyProvider = &types.KeyProviderId{
			Id: providerID,
		}
	}
	res, err := methods.GenerateKey(ctx, m.Client(), &req)
	if err != nil {
		return "", err
	}
	if !res.Returnval.Success {
		err := generateKeyError{reason: res.Returnval.Reason}
		if res.Returnval.Fault != nil {
			err.LocalizedMethodFault = *res.Returnval.Fault
		}
		return "", err
	}
	return res.Returnval.KeyId.KeyId, nil
}

type generateKeyError struct {
	types.LocalizedMethodFault
	reason string
}

func (e generateKeyError) Error() string {

	return e.reason
}

func (e generateKeyError) GetLocalizedMethodFault() *types.LocalizedMethodFault {
	return &e.LocalizedMethodFault
}
