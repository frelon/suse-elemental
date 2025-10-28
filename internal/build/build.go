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
	"context"
	"fmt"
	"strings"

	"github.com/suse/elemental/v3/internal/image"
	imginstall "github.com/suse/elemental/v3/internal/image/install"
	"github.com/suse/elemental/v3/internal/manifest/extractor"
	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/firmware"
	"github.com/suse/elemental/v3/pkg/install"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/manifest/source"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/unpack"
	"github.com/suse/elemental/v3/pkg/upgrade"
)

type downloadFunc func(ctx context.Context, fs vfs.FS, url, path string) error

type helmConfigurator interface {
	Configure(definition *image.Definition, manifest *resolver.ResolvedManifest) ([]string, error)
}

type Builder struct {
	System       *sys.System
	Helm         helmConfigurator
	DownloadFile downloadFunc
	Local        bool
	ConfigDir    image.ConfigDir
}

func (b *Builder) Run(ctx context.Context, d *image.Definition, buildDir image.BuildDir) error {
	logger := b.System.Logger()
	runner := b.System.Runner()
	fs := b.System.FS()

	logger.Info("Resolving release manifest: %s", d.Release.ManifestURI)
	m, err := resolveManifest(fs, d.Release.ManifestURI, buildDir, b.Local)
	if err != nil {
		logger.Error("Resolving release manifest failed")
		return err
	}

	preparePart := b.generatePreparePartition(d)
	if preparePart != nil {
		if err := b.configureNetworkOnPartition(d, buildDir, preparePart); err != nil {
			logger.Error("Configuring network failed")
			return err
		}
	}

	k8sScript, k8sConfScript, err := b.configureKubernetes(ctx, d, m, buildDir)
	if err != nil {
		logger.Error("Configuring Kubernetes failed")
		return err
	}

	if k8sScript != "" || len(d.ButaneConfig) > 0 || k8sConfScript != "" {
		if err = b.configureIgnition(d, buildDir, k8sScript, k8sConfScript); err != nil {
			logger.Error("Configuring Ignition failed")
			return err
		}
	}

	if err = b.downloadSystemExtensions(ctx, d, m, buildDir); err != nil {
		logger.Error("Downloading system extensions failed")
		return err
	}

	logger.Info("Creating RAW disk image")
	if err = createDisk(runner, d.Image, d.Installation.DiskSize); err != nil {
		logger.Error("Creating RAW disk image failed")
		return err
	}

	logger.Info("Attaching loop device to RAW disk image")
	device, err := attachDevice(runner, d.Image)
	if err != nil {
		logger.Error("Attaching loop device failed")
		return err
	}
	defer func() {
		if dErr := detachDevice(runner, device); dErr != nil {
			logger.Error("Detaching loop device failed: %v", dErr)
		}
	}()

	err = vfs.MkdirAll(b.System.FS(), buildDir.OverlaysDir(), vfs.DirPerm)
	if err != nil {
		logger.Error("Failed creating overlay dir")
		return err
	}

	logger.Info("Preparing installation setup")
	dep, err := newDeployment(
		b.System,
		device,
		d.Installation.Bootloader,
		d.Installation.KernelCmdLine,
		m.CorePlatform.Components.OperatingSystem.Image,
		buildDir,
		preparePart,
	)
	if err != nil {
		logger.Error("Preparing installation setup failed")
		return err
	}

	boot, err := bootloader.New(dep.BootConfig.Bootloader, b.System)
	if err != nil {
		logger.Error("Parsing boot config failed")
		return err
	}

	manager := firmware.NewEfiBootManager(b.System)
	upgrader := upgrade.New(
		ctx, b.System, upgrade.WithBootManager(manager), upgrade.WithBootloader(boot),
		upgrade.WithUnpackOpts(unpack.WithLocal(b.Local)),
	)
	installer := install.New(ctx, b.System, install.WithUpgrader(upgrader))

	logger.Info("Installing OS")
	if err = installer.Install(dep); err != nil {
		logger.Error("Installation failed")
		return err
	}

	logger.Info("Installation complete")

	return nil
}

func newDeployment(system *sys.System, installationDevice, bootloader, kernelCmdLine, osImage string, buildDir image.BuildDir, customPartitions ...*deployment.Partition) (*deployment.Deployment, error) {
	var d *deployment.Deployment
	if ok, _ := vfs.Exists(system.FS(), buildDir.FirstbootConfigDir()); ok {
		configSize, err := vfs.DirSizeMB(system.FS(), buildDir.FirstbootConfigDir())
		if err != nil {
			return nil, fmt.Errorf("failed to compute configuration partition size: %w", err)
		}
		d = deployment.New(
			deployment.WithPartitions(1, customPartitions...),
			deployment.WithConfigPartition(deployment.MiB(configSize)),
		)
	} else {
		d = deployment.New(
			deployment.WithPartitions(1, customPartitions...),
		)
	}

	d.Disks[0].Device = installationDevice
	d.BootConfig.Bootloader = bootloader
	d.BootConfig.KernelCmdline = kernelCmdLine

	osURI := fmt.Sprintf("%s://%s", deployment.OCI, osImage)
	osSource, err := deployment.NewSrcFromURI(osURI)
	if err != nil {
		return nil, fmt.Errorf("parsing OS source URI %q: %w", osURI, err)
	}
	d.SourceOS = osSource

	overlaysURI := fmt.Sprintf("%s://%s", deployment.Dir, buildDir.OverlaysDir())
	overlaySource, err := deployment.NewSrcFromURI(overlaysURI)
	if err != nil {
		return nil, fmt.Errorf("parsing overlay source URI %q: %w", overlaysURI, err)
	}
	d.OverlayTree = overlaySource

	if err = d.Sanitize(system); err != nil {
		return nil, fmt.Errorf("sanitizing deployment: %w", err)
	}

	return d, nil
}

func resolveManifest(fs vfs.FS, manifestURI string, buildDir image.BuildDir, local bool) (*resolver.ResolvedManifest, error) {
	manifestsDir := buildDir.ReleaseManifestsDir()
	if err := vfs.MkdirAll(fs, manifestsDir, 0700); err != nil {
		return nil, fmt.Errorf("creating release manifest store '%s': %w", manifestsDir, err)
	}

	extr, err := extractor.New(extractor.WithStore(manifestsDir))
	if err != nil {
		return nil, fmt.Errorf("initialising OCI release manifest extractor: %w", err)
	}

	res := resolver.New(source.NewReader(extr, local))
	m, err := res.Resolve(manifestURI)
	if err != nil {
		return nil, fmt.Errorf("resolving manifest at uri '%s': %w", manifestURI, err)
	}

	return m, nil
}

func createDisk(runner sys.Runner, img image.Image, diskSize imginstall.DiskSize) error {
	const defaultSize = "10G"

	if diskSize == "" {
		diskSize = defaultSize
	} else if !diskSize.IsValid() {
		return fmt.Errorf("invalid disk size definition '%s'", diskSize)
	}

	_, err := runner.Run("qemu-img", "create", "-f", "raw", img.OutputImageName, string(diskSize))
	return err
}

func attachDevice(runner sys.Runner, img image.Image) (string, error) {
	out, err := runner.Run("losetup", "-f", "--show", img.OutputImageName)
	if err != nil {
		return "", err
	}

	device := strings.TrimSpace(string(out))
	return device, nil
}

func detachDevice(runner sys.Runner, device string) error {
	_, err := runner.Run("losetup", "-d", device)
	return err
}
