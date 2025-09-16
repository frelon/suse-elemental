# Firstboot OS Configuration

This section provides an overview of how to configure the OS at firstboot.

## The runtime context

Unified Core images crafted by `elemental3ctl` include, by default, two partitions, an ESP partition
(bootloader, kernel and initrd) and a Linux partition (the OS itself). The OS partition is a btrfs
partition including several subvolumes where there is the default read-only subvolume mounted as the
root device and a list of read-write subvolumes which are mounted in paths that are expected or required
to be read-write, such as `/etc` or `/var`. The default subvolume is a btrfs read-only subvolume so
regardless of mounting it with or without read-write capabilities, the filesystem will prevent any write
in there causing a failure in any attempt.

Default read-write subvolumes for unified core images are:

* /var
* /root
* /etc
* /opt
* /srv
* /home

The only subvolumes that are mounted in early boot inside initrd before switching root are `/etc`, `/root`
and `/var`. This is relevant for first boot configuration as these are the subvolumes that tools such as
Ignition are capable of modifying at boot time. Those are the subvolumes that include the `x-initrd.mount`
option in `/etc/fstab` file, stating those are mounted before switching root.

Despite this defaults elemental3ctl is capable of setting additional partitions and different subvolumes.
`elemental3ctl` supports installation parameters provided by a yaml file, the default disk setup is
equivalent to the following one:

```yaml
disks:
- partitions:
  - label: EFI
    role: efi
    mountPoint: /boot
    size: 1024
    mountOpts: ['defaults', 'x-systemd.automount']
  - label: SYSTEM
    role: system
    mountPoint: /
    size: 0
    filesystem: btrfs
    rwVolumes:
    - path: /var
      noCopyOnWrite: true
      mountOpts: ['x-initrd.mount']
    - path: /root
      mountOpts: ['x-initrd.mount']
    - path: /etc
      snapshotted: true
      mountOpts: ['x-initrd.mount']
    - path: /opt
    - path: /srv
    - path: /home
```

Which could be adapted to include an additional partition by adding another item into the partitions
list. Adding an Ignition partition of 512MiB could done with:

```yaml
disks:
- target: /dev/loop0
  partitions:
  - label: EFI
    role: efi
    mountPoint: /boot
    size: 1024
    mountOpts: ['defaults', 'x-systemd.automount']
  - label: IGNITION
    role: data
    mountPoint: /firstboot
    size: 512
    filesystem: btrfs
    mountOpts: ['defaults', 'x-systemd.automount']
  - label: SYSTEM
    role: system
    mountPoint: /
    size: 0
    filesystem: btrfs
    rwVolumes:
    - path: /var
      noCopyOnWrite: true
      mountOpts: ['x-initrd.mount']
    - path: /root
      mountOpts: ['x-initrd.mount']
    - path: /etc
      snapshotted: true
      mountOpts: ['x-initrd.mount']
    - path: /opt
    - path: /srv
    - path: /home
```

## Configuring via Ignition

Ignition is functional and supported in SUSE Linux Micro, however it comes with certain constraints when this
is used in conjunction to an image based (also referred as immutable) OS. Most noticeable aspect is that the
root volume, despite ignition attempts to remount it as a read-write, is still sealed and operating in read-only
mode. In practice this essentially means that changes over the RO areas of the system are forbidden and any
attempt to write there is leading to an ignition failure which translates into a non booting system.

The Ignition configuration file can be provided in a variety of ways depending on the platform (e.g. some
public cloud provider) the system is running, however a common and easy way to provide the configuration is by using a
volume with a filesystem labeled `IGNITION` (not key sensitive), containing a configuration file stored at the
`/ignition/config.ign` path. This can be achieved by adding an extra block device,
such as a USB stick, to the machine, or by installing a system including a partition with a filesystem labeled as `IGNITION`.

### Ignition configuration example

The configuration example below demonstrates the common use cases of Ignition, such as configuring users, dropping
files into the system and handling systemd services. Find full documentation about Ignition at the
[official website](https://coreos.github.io/ignition/) of the project.

```json
{
  "ignition": { "version": "3.5.0" },
  "passwd": {
    "users": [
      {
        "name": "root",
        "passwordHash": "$6$6RmQVwdZ3p0TLx8t$e1RdFSCTNIJRe9fDGnssaQIrTblBbApEE8PCu4FGS/1/PToT9g/GDT05RSF.Ijm6wKs8m3mApYPMw/.oUc0MS0",
        "sshAuthorizedKeys": ["ssh-rsa veryLongRSAPublicKey"]
      },
      {
        "name": "pipo",
        "passwordHash": "$6$6RmQVwdZ3p0TLx8t$e1RdFSCTNIJRe9fDGnssaQIrTblBbApEE8PCu4FGS/1/PToT9g/GDT05RSF.Ijm6wKs8m3mApYPMw/.oUc0MS0",
        "system": false,
        "noCreateHome": true
      }
    ]
  },
  "systemd": {
    "units": [
      {
        "name": "example.service",
        "enabled": true,
        "contents": "[Service]\nType=oneshot\nExecStart=/usr/bin/echo Hello World\n\n[Install]\nWantedBy=multi-user.target"
      }
    ]
  },
  "storage": {
    "files": [
      {
        "path": "/var/test.txt",
        "mode": 420,
        "contents": {
          "source": "data:,testcontents"
        },
        "overwrite": true
      },
      {
        "path": "/etc/subdir/test.txt",
        "mode": 420,
        "contents": {
          "source": "data:,test%20contents%20in%20etc"
        },
        "overwrite": true
      }
    ],
  }
}
```

Note the `pipo` user is added with the `noCreateHome` parameter, that is because `/home`, by default, is not including
the `x-initrd.mount` option in its volume definition, meaning it is not mounted at the time ignition is executed and
that `/home` is a RO filesystem.
