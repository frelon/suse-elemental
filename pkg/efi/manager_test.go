/*
Copyright © 2021 - 2025 SUSE LLC

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

package efi_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/efi"
	"github.com/suse/elemental/v3/pkg/log"

	"github.com/suse/elemental/v3/pkg/efi/mock"
)

func TestEFISuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EFI test suite")
}

var _ = Describe("EFI Manager", Label("efi", "manager"), func() {
	var logger log.Logger

	BeforeEach(func() {
		logger = log.New(log.WithDiscardAll())
	})

	It("creates a BootManager without error", func() {
		var vars efi.Variables = mock.NewMockEFIVariables()

		manager, err := efi.NewBootManagerForVariables(nil, vars)
		Expect(err).To(BeNil())

		Expect(manager).ToNot(BeNil())
	})

	It("creates a BootManager with ReadLoadOptions error", func() {
		var vars efi.Variables = mock.NewMockEFIVariables().WithLoadOptionError(fmt.Errorf("cannot read device path: cannot decode node: 1: invalid length 14 bytes (too large)"))

		err := vars.WriteVariable("BootOrder", efi.GlobalVariable, efi.AttributeNonVolatile, []byte("0001"))
		Expect(err).To(BeNil())

		err = vars.WriteVariable("Boot0001", efi.GlobalVariable, efi.AttributeNonVolatile, []byte("test.efi"))
		Expect(err).To(BeNil())

		manager, err := efi.NewBootManagerForVariables(logger, vars)
		Expect(err).To(BeNil())

		Expect(manager).ToNot(BeNil())
	})
})
