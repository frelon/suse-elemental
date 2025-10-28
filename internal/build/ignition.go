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
	_ "embed"
	"fmt"
	"path/filepath"

	"github.com/coreos/butane/base/v0_6"
	"github.com/coreos/ignition/v2/config/util"
	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/internal/butane"
	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/template"
	"github.com/suse/elemental/v3/pkg/sys"
)

const (
	ensureSysextUnitName = "ensure-sysext.service"
	k8sResourcesUnitName = "k8s-resource-installer.service"
	k8sConfigUnitName    = "k8s-config-installer.service"
)

//go:embed templates/ensure-sysext.service
var ensureSysextUnit string

//go:embed templates/k8s-resource-installer.service.tpl
var k8sResourceUnitTpl string

//go:embed templates/k8s-config-installer.service.tpl
var k8sConfigUnitTpl string

// configureIngition writes the ignition configuration file based on the provided butane configuration
// and the given kubernetes configuration
func (b *Builder) configureIgnition(def *image.Definition, buildDir image.BuildDir, k8sScript, k8sConfScript string) error {
	if len(def.ButaneConfig) == 0 && k8sScript == "" && k8sConfScript == "" {
		b.System.Logger().Info("No ignition configuration required")
		return nil
	}

	const (
		variant = "fcos"
		version = "1.6.0"
	)
	var config butane.Config

	config.Variant = variant
	config.Version = version

	if len(def.ButaneConfig) > 0 {
		b.System.Logger().Info("Translating butane configuration to Ignition syntax")

		ignitionBytes, err := butane.TranslateBytes(b.System, def.ButaneConfig)
		if err != nil {
			return fmt.Errorf("failed translating butane configuration: %w", err)
		}
		config.MergeInlineIgnition(string(ignitionBytes))
	} else {
		b.System.Logger().Info("No butane configuration to translate into Ignition syntax")
	}

	if k8sScript != "" {
		k8sResourcesUnit, err := generateK8sResourcesUnit(k8sScript)
		if err != nil {
			return err
		}

		config.AddSystemdUnit(ensureSysextUnitName, ensureSysextUnit, true)
		config.AddSystemdUnit(k8sResourcesUnitName, k8sResourcesUnit, true)
	}

	if k8sConfScript != "" {
		err := appendRke2Configuration(b.System, &config, &def.Kubernetes, b.ConfigDir, k8sConfScript)
		if err != nil {
			return fmt.Errorf("failed appending rke2 configuration: %w", err)
		}
	}

	ignitionFile := filepath.Join(buildDir.FirstbootConfigDir(), image.IgnitionFilePath())
	return butane.WriteIgnitionFile(b.System, config, ignitionFile)
}

func generateK8sResourcesUnit(deployScript string) (string, error) {
	values := struct {
		KubernetesDir        string
		ManifestDeployScript string
	}{
		KubernetesDir:        filepath.Dir(deployScript),
		ManifestDeployScript: deployScript,
	}

	data, err := template.Parse(k8sResourcesUnitName, k8sResourceUnitTpl, &values)
	if err != nil {
		return "", fmt.Errorf("parsing config script template: %w", err)
	}
	return data, nil
}

func generateK8sConfigUnit(deployScript string) (string, error) {
	values := struct {
		ConfigDeployScript string
	}{
		ConfigDeployScript: deployScript,
	}

	data, err := template.Parse(k8sConfigUnitName, k8sConfigUnitTpl, &values)
	if err != nil {
		return "", fmt.Errorf("parsing config script template: %w", err)
	}
	return data, nil
}

func appendRke2Configuration(s *sys.System, config *butane.Config, k *kubernetes.Kubernetes, configDir image.ConfigDir, configScript string) error {
	c, err := NewCluster(s, k, configDir.KubernetesConfigDir())
	if err != nil {
		return fmt.Errorf("failed parsing cluster: %w", err)
	}

	k8sConfigUnit, err := generateK8sConfigUnit(configScript)
	if err != nil {
		return fmt.Errorf("failed generating k8s config unit: %w", err)
	}

	config.AddSystemdUnit(k8sConfigUnitName, k8sConfigUnit, true)

	targetDir := "/etc/rancher/rke2"

	serverBytes, err := marshalConfig(c.ServerConfig)
	if err != nil {
		return fmt.Errorf("failed marshaling server config: %w", err)
	}

	config.Storage.Files = append(config.Storage.Files, v0_6.File{
		Path:     filepath.Join(targetDir, "server.yaml"),
		Contents: v0_6.Resource{Inline: util.StrToPtr(string(serverBytes))},
	})

	agentBytes, err := marshalConfig(c.AgentConfig)
	if err != nil {
		return fmt.Errorf("failed marshaling agent config: %w", err)
	}

	config.Storage.Files = append(config.Storage.Files, v0_6.File{
		Path:     filepath.Join(targetDir, "agent.yaml"),
		Contents: v0_6.Resource{Inline: util.StrToPtr(string(agentBytes))},
	})

	return nil
}

func marshalConfig(config map[string]any) ([]byte, error) {
	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("serializing kubernetes config: %w", err)
	}

	return data, nil
}
