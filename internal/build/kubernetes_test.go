/*
Copyright © 2025 SUSE LLC
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

package build

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/image/release"
)

var _ = Describe("Kubernetes tests", func() {
	Describe("Resources trigger", func() {
		It("Skips manifests setup if manifests are not provided", func() {
			k := &kubernetes.Kubernetes{}
			Expect(needsManifestsSetup(k)).To(BeFalse())
		})

		It("Requires manifests setup if local manifests are provided", func() {
			k := &kubernetes.Kubernetes{
				LocalManifests: []string{"/apache.yaml"},
			}
			Expect(needsManifestsSetup(k)).To(BeTrue())
		})

		It("Requires manifests setup if remote manifests are provided", func() {
			k := &kubernetes.Kubernetes{
				RemoteManifests: []string{"https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.31/deploy/local-path-storage.yaml"},
			}
			Expect(needsManifestsSetup(k)).To(BeTrue())
		})

		It("Skips Helm setup if charts are not provided", func() {
			def := &image.Definition{}
			Expect(needsHelmChartsSetup(def)).To(BeFalse())
		})

		It("Requires Helm setup if user charts are provided", func() {
			def := &image.Definition{
				Kubernetes: kubernetes.Kubernetes{
					Helm: &kubernetes.Helm{
						Charts: []*kubernetes.HelmChart{
							{Name: "apache", RepositoryName: "apache-repo"},
						},
					},
				},
			}
			Expect(needsHelmChartsSetup(def)).To(BeTrue())
		})

		It("Requires Helm setup if core charts are provided", func() {
			def := &image.Definition{
				Release: release.Release{
					Core: release.Components{
						Helm: []release.HelmChart{
							{
								Name: "metallb",
							},
						},
					},
				},
			}

			Expect(needsHelmChartsSetup(def)).To(BeTrue())
		})

		It("Requires Helm setup if product charts are provided", func() {
			def := &image.Definition{
				Release: release.Release{
					Product: release.Components{
						Helm: []release.HelmChart{
							{
								Name: "rancher",
							},
						},
					},
				},
			}

			Expect(needsHelmChartsSetup(def)).To(BeTrue())
		})
	})
})
