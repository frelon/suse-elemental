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

package action

import (
	"fmt"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"
	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/internal/cli/cmd"
	"github.com/suse/elemental/v3/pkg/bootloader"
	"github.com/suse/elemental/v3/pkg/deployment"
	"github.com/suse/elemental/v3/pkg/firmware"
	"github.com/suse/elemental/v3/pkg/install"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
	"github.com/suse/elemental/v3/pkg/transaction"
	"github.com/suse/elemental/v3/pkg/unpack"
	"github.com/suse/elemental/v3/pkg/upgrade"
)

func Install(ctx *cli.Context) error { //nolint:dupl
	var s *sys.System
	args := &cmd.InstallArgs
	if ctx.App.Metadata == nil || ctx.App.Metadata["system"] == nil {
		return fmt.Errorf("error setting up initial configuration")
	}
	s = ctx.App.Metadata["system"].(*sys.System)

	s.Logger().Info("Starting install action with args: %+v", args)

	d, err := digestInstallSetup(s, args)
	if err != nil {
		s.Logger().Error("Failed to collect installation setup")
		return err
	}

	s.Logger().Info("Checked configuration, running installation process")

	ctxCancel, stop := signal.NotifyContext(ctx.Context, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		stop()
	}()

	bootloader, err := bootloader.New(d.BootConfig.Bootloader, s)
	if err != nil {
		s.Logger().Error("Parsing boot config failed")
		return err
	}

	snapshotter, err := transaction.New(ctxCancel, s, d, d.Snapshotter.Name)
	if err != nil {
		s.Logger().Error("Parsing snapshotter config failed")
		return err
	}

	manager := firmware.NewEfiBootManager(s)
	upgrader := upgrade.New(
		ctxCancel, s, upgrade.WithBootManager(manager), upgrade.WithBootloader(bootloader),
		upgrade.WithSnapshotter(snapshotter),
		upgrade.WithUnpackOpts(unpack.WithVerify(args.Verify), unpack.WithLocal(args.Local)),
	)
	installer := install.New(ctxCancel, s, install.WithUpgrader(upgrader))

	err = installer.Install(d)
	if err != nil {
		s.Logger().Error("Installation failed")
		return err
	}

	s.Logger().Info("Installation complete")

	return nil
}

func digestInstallSetup(s *sys.System, flags *cmd.InstallFlags) (*deployment.Deployment, error) {
	d := deployment.DefaultDeployment()
	if flags.Description != "" {
		if ok, _ := vfs.Exists(s.FS(), flags.Description); !ok {
			return nil, fmt.Errorf("config file '%s' not found", flags.Description)
		}
		data, err := s.FS().ReadFile(flags.Description)
		if err != nil {
			return nil, fmt.Errorf("could not read description file '%s': %w", flags.Description, err)
		}
		err = yaml.Unmarshal(data, d)
		if err != nil {
			return nil, fmt.Errorf("could not unmarshal config file: %w", err)
		}
	}
	if flags.Target != "" && len(d.Disks) > 0 {
		d.Disks[0].Device = flags.Target
	}

	if flags.OperatingSystemImage != "" {
		srcOS, err := deployment.NewSrcFromURI(flags.OperatingSystemImage)
		if err != nil {
			return nil, fmt.Errorf("failed parsing OS source URI ('%s'): %w", flags.OperatingSystemImage, err)
		}
		d.SourceOS = srcOS
	}

	if flags.Overlay != "" {
		overlay, err := deployment.NewSrcFromURI(flags.Overlay)
		if err != nil {
			return nil, fmt.Errorf("failed parsing overlay source URI ('%s'): %w", flags.Overlay, err)
		}
		d.OverlayTree = overlay
	}

	if flags.ConfigScript != "" {
		d.CfgScript = flags.ConfigScript
	}

	if flags.CreateBootEntry {
		d.Firmware.BootEntries = []*firmware.EfiBootEntry{
			firmware.DefaultBootEntry(s.Platform(), d.Disks[0].Device),
		}
	}

	if flags.Bootloader != bootloader.BootNone {
		d.BootConfig.Bootloader = flags.Bootloader
	}

	if flags.KernelCmdline != "" {
		d.BootConfig.KernelCmdline = fmt.Sprintf("%s %s", d.BootConfig.KernelCmdline, flags.KernelCmdline)
	}

	if flags.EnableFips {
		d.Fips = &deployment.FipsConfig{
			Enabled: true,
		}

		bootFlag := fmt.Sprintf("boot=LABEL=%s", deployment.EfiLabel)
		d.BootConfig.KernelCmdline = fmt.Sprintf("%s %s %s", d.BootConfig.KernelCmdline, "fips=1", bootFlag)
	}

	if flags.Snapshotter != "" {
		d.Snapshotter.Name = flags.Snapshotter

		if d.Snapshotter.Name == "overwrite" {
			s.Logger().Warn("'overwrite' snapshotter is a debugging tool and should not be used for production installation")

			sysPart := d.GetSystemPartition()
			if sysPart != nil {
				sysPart.FileSystem = deployment.Ext4
				sysPart.RWVolumes = nil
			}
		}
	}

	err := d.Sanitize(s)
	if err != nil {
		return nil, fmt.Errorf("inconsistent deployment setup found: %w", err)
	}

	return d, nil
}
