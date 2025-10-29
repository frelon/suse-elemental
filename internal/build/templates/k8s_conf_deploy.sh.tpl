#!/bin/bash

set -uo pipefail

declare -A hosts

{{- range .Nodes }}
hosts[{{ .Hostname }}]={{ .Type }}
{{- end }}

HOSTNAME=$(cat /etc/hostname)
if [ ! "$HOSTNAME" ]; then
    HOSTNAME=$(cat /proc/sys/kernel/hostname)
fi

NODETYPE="${hosts[$HOSTNAME]:-server}"

CONFIGFILE="{{ .KubernetesDir }}/$NODETYPE.yaml"
mkdir -p /etc/rancher/rke2
cp $CONFIGFILE /etc/rancher/rke2/config.yaml

{{- if and .APIVIP4 .APIHost }}
echo "{{ .APIVIP4 }} {{ .APIHost }}" >> /etc/hosts
{{- end }}

{{- if and .APIVIP6 .APIHost }}
echo "{{ .APIVIP6 }} {{ .APIHost }}" >> /etc/hosts
{{- end }}

