/*
Copyright © 2021 - 2025 SUSE LLC
SPDX-License-Identifier: Apache-2.0

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

package efi

import (
	"io"

	efi "github.com/canonical/go-efilib"
	efi_linux "github.com/canonical/go-efilib/linux"
)

type (
	VariableDescriptor       = efi.VariableDescriptor
	VariableAttributes       = efi.VariableAttributes
	GUID                     = efi.GUID
	LoadOption               = efi.LoadOption
	DevicePath               = efi.DevicePath
	FilePathToDevicePathMode = efi_linux.FilePathToDevicePathMode
)

var (
	ErrVarsUnavailable = efi.ErrVarsUnavailable
	ErrVarNotExist     = efi.ErrVarNotExist
	ErrVarPermission   = efi.ErrVarPermission

	GlobalVariable       = efi.GlobalVariable
	AttributeNonVolatile = efi.AttributeNonVolatile

	NewFilePathDevicePathNode = efi.NewFilePathDevicePathNode
)

type Variables interface {
	ListVariables() ([]VariableDescriptor, error)
	GetVariable(guid GUID, name string) (data []byte, attrs VariableAttributes, err error)
	WriteVariable(name string, guid GUID, attrs VariableAttributes, data []byte) error
	NewFileDevicePath(filepath string, mode FilePathToDevicePathMode) (DevicePath, error)
	DelVariable(guid GUID, name string) error
	ReadLoadOption(r io.Reader) (out *LoadOption, err error)
}

type Vars struct{}

var _ Variables = (*Vars)(nil)

func (v Vars) DelVariable(guid GUID, name string) error {
	_, attrs, err := v.GetVariable(guid, name)
	if err != nil {
		return err
	}

	return v.WriteVariable(name, guid, attrs, nil)
}

func (v Vars) NewFileDevicePath(filepath string, mode efi_linux.FilePathToDevicePathMode) (efi.DevicePath, error) {
	return efi_linux.FilePathToDevicePath(filepath, mode)
}

func (Vars) ListVariables() ([]efi.VariableDescriptor, error) {
	return efi.ListVariables(efi.DefaultVarContext)
}

func (Vars) GetVariable(guid efi.GUID, name string) (data []byte, attrs efi.VariableAttributes, err error) {
	return efi.ReadVariable(efi.DefaultVarContext, name, guid)
}

func (Vars) WriteVariable(name string, guid GUID, attrs VariableAttributes, data []byte) error {
	return efi.WriteVariable(efi.DefaultVarContext, name, guid, attrs, data)
}

func (Vars) ReadLoadOption(r io.Reader) (out *efi.LoadOption, err error) {
	return efi.ReadLoadOption(r)
}

type BootManager struct {
	efivars        Variables
	entries        map[uint16]BootEntryVariable
	bootOrder      []uint16
	bootOrderAttrs VariableAttributes
}

type BootEntryVariable struct {
	BootNumber uint16
	Data       []byte
	Attributes VariableAttributes
	LoadOption *LoadOption
}

type BootEntry struct {
	Filename    string
	Label       string
	Options     string
	Description string
}
