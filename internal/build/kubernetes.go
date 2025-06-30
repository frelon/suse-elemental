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
	_ "embed"
	"fmt"
	"path/filepath"

	"github.com/suse/elemental/v3/internal/image"
	"github.com/suse/elemental/v3/internal/image/kubernetes"
	"github.com/suse/elemental/v3/internal/template"
	"github.com/suse/elemental/v3/pkg/manifest/resolver"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

//go:embed templates/k8s_res_deploy.sh.tpl
var k8sResDeployScriptTpl string

func needsManifestsSetup(k *kubernetes.Kubernetes) bool {
	return len(k.RemoteManifests) > 0 || len(k.LocalManifests) > 0
}

func (b *Builder) configureKubernetes(
	ctx context.Context,
	def *image.Definition,
	manifest *resolver.ResolvedManifest,
	buildDir image.BuildDir,
) (k8sResourceScript string, err error) {
	b.System.Logger().Info("Downloading RKE2 extension")

	extensionsDir := filepath.Join(buildDir.OverlaysDir(), "var", "lib", "extensions")
	if err = vfs.MkdirAll(b.System.FS(), extensionsDir, 0o700); err != nil {
		return "", fmt.Errorf("creating extensions directory: %w", err)
	}

	rke2URL := manifest.CorePlatform.Components.Kubernetes.RKE2.Image
	rke2Extension := filepath.Join(extensionsDir, filepath.Base(rke2URL))

	if err = b.DownloadFile(ctx, b.System.FS(), rke2URL, rke2Extension); err != nil {
		return "", fmt.Errorf("downloading RKE2 extension %q: %w", rke2URL, err)
	}

	var runtimeHelmCharts []string
	if needsHelmChartsSetup(def) {
		if runtimeHelmCharts, err = b.Helm.Configure(def, manifest); err != nil {
			return "", fmt.Errorf("configuring helm charts: %w", err)
		}
	}

	var runtimeManifestsDir string
	if needsManifestsSetup(&def.Kubernetes) {
		relativeManifestsPath := image.KubernetesManifestsPath()
		manifestsOverlayPath := filepath.Join(buildDir.OverlaysDir(), relativeManifestsPath)
		if err = b.setupManifests(ctx, &def.Kubernetes, manifestsOverlayPath); err != nil {
			return "", fmt.Errorf("configuring kubernetes manifests: %w", err)
		}

		runtimeManifestsDir = filepath.Join("/", relativeManifestsPath)
	}

	if len(runtimeHelmCharts) > 0 || runtimeManifestsDir != "" {
		relativeK8sPath := image.KubernetesPath()
		kubernetesOverlayPath := filepath.Join(buildDir.OverlaysDir(), relativeK8sPath)
		scriptInOverlay, err := writeK8sResDeployScript(b.System.FS(), kubernetesOverlayPath, runtimeManifestsDir, runtimeHelmCharts)
		if err != nil {
			return "", fmt.Errorf("writing kubernetes resource deployment script: %w", err)
		}

		k8sResourceScript = filepath.Join("/", relativeK8sPath, filepath.Base(scriptInOverlay))
	}

	return k8sResourceScript, nil
}

func (b *Builder) setupManifests(ctx context.Context, k *kubernetes.Kubernetes, manifestsDir string) error {
	fs := b.System.FS()
	if err := vfs.MkdirAll(fs, manifestsDir, vfs.DirPerm); err != nil {
		return fmt.Errorf("setting up manifests directory '%s': %w", manifestsDir, err)
	}

	for _, manifest := range k.RemoteManifests {
		path := filepath.Join(manifestsDir, filepath.Base(manifest))

		if err := b.DownloadFile(ctx, fs, manifest, path); err != nil {
			return fmt.Errorf("downloading remote Kubernetes manifest '%s': %w", manifest, err)
		}
	}

	for _, manifest := range k.LocalManifests {
		overlayPath := filepath.Join(manifestsDir, filepath.Base(manifest))
		if err := vfs.CopyFile(fs, manifest, overlayPath); err != nil {
			return fmt.Errorf("copying local manifest '%s' to '%s': %w", manifest, overlayPath, err)
		}
	}

	return nil
}

func writeK8sResDeployScript(fs vfs.FS, dest, runtimeManifestsDir string, runtimeHelmCharts []string) (path string, err error) {
	const k8sResDeployScriptName = "k8s_res_deploy.sh"

	values := struct {
		HelmCharts   []string
		ManifestsDir string
	}{
		HelmCharts:   runtimeHelmCharts,
		ManifestsDir: runtimeManifestsDir,
	}

	data, err := template.Parse(k8sResDeployScriptName, k8sResDeployScriptTpl, &values)
	if err != nil {
		return "", fmt.Errorf("parsing template for %s: %w", k8sResDeployScriptName, err)
	}

	filename := filepath.Join(dest, k8sResDeployScriptName)
	if err = fs.WriteFile(filename, []byte(data), 0o744); err != nil {
		return "", fmt.Errorf("writing %s: %w", filename, err)
	}
	return filename, nil
}
