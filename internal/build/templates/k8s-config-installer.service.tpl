[Unit]
Description=Kubernetes Config Installer
ConditionPathExists=!/etc/rancher/rke2/config.yaml
Wants=network.target
After=network.target
Before=network-online.target

[Service]
Type=oneshot
TimeoutSec=900
Restart=on-failure
RestartSec=60
ExecStart=/bin/bash "{{ .ConfigDeployScript }}" 
ExecStartPost=/bin/sh -c "systemctl disable k8s-config-installer.service"
ExecStartPost=/bin/sh -c "rm -rf /etc/systemd/system/k8s-config-installer.service"

[Install]
WantedBy=multi-user.target
