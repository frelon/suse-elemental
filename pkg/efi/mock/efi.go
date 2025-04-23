/*
Copyright © 2022 - 2025 SUSE LLC
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

package mock

import (
	"io"
	"strings"

	"github.com/twpayne/go-vfs/v4"

	"github.com/suse/elemental/v3/pkg/efi"
)

type mockEFIVariable struct {
	data  []byte
	attrs efi.VariableAttributes
}

// EFIVariables implements an in-memory variable store.
type EFIVariables struct {
	store         map[efi.VariableDescriptor]mockEFIVariable
	loadOptionErr error
}

var _ efi.Variables = (*EFIVariables)(nil)

func NewMockEFIVariables() *EFIVariables {
	return &EFIVariables{
		store: make(map[efi.VariableDescriptor]mockEFIVariable),
	}
}

func (m *EFIVariables) WithLoadOptionError(err error) *EFIVariables {
	m.loadOptionErr = err
	return m
}

func (m EFIVariables) DelVariable(_ efi.GUID, _ string) error {
	return nil
}

// ListVariables implements EFIVariables
func (m EFIVariables) ListVariables() (out []efi.VariableDescriptor, err error) {
	for k := range m.store {
		out = append(out, k)
	}
	return out, nil
}

// GetVariable implements EFIVariables
func (m EFIVariables) GetVariable(guid efi.GUID, name string) (data []byte, attrs efi.VariableAttributes, err error) {
	out, ok := m.store[efi.VariableDescriptor{Name: name, GUID: guid}]
	if !ok {
		return nil, 0, efi.ErrVarNotExist
	}
	return out.data, out.attrs, nil
}

// WriteVariable implements EFIVariables
func (m EFIVariables) WriteVariable(name string, guid efi.GUID, attrs efi.VariableAttributes, data []byte) error {
	if len(data) == 0 {
		delete(m.store, efi.VariableDescriptor{Name: name, GUID: guid})
	} else {
		m.store[efi.VariableDescriptor{Name: name, GUID: guid}] = mockEFIVariable{data, attrs}
	}
	return nil
}

func (m EFIVariables) ReadLoadOption(_ io.Reader) (out *efi.LoadOption, err error) {
	return nil, m.loadOptionErr
}

func (m EFIVariables) NewFileDevicePath(fpath string, _ efi.FilePathToDevicePathMode) (efi.DevicePath, error) {
	file, err := vfs.OSFS.Open(fpath)
	if err != nil {
		return nil, err
	}
	file.Close()

	const espLocation = "/boot/efi/"
	fpath = strings.TrimPrefix(fpath, espLocation)

	return efi.DevicePath{
		efi.NewFilePathDevicePathNode(fpath),
	}, nil
}
