[![Build Status](https://gitlab.com/Northern.tech/Mender/mender-artifact/badges/master/pipeline.svg)](https://gitlab.com/Northern.tech/Mender/mender-artifact/pipelines)
[![Coverage Status](https://coveralls.io/repos/github/mendersoftware/mender-artifact/badge.svg?branch=master)](https://coveralls.io/github/mendersoftware/mender-artifact?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/mendersoftware/mender-artifact)](https://goreportcard.com/report/github.com/mendersoftware/mender-artifact)

Mender Artifacts Library
==============================================

Mender is an open source over-the-air (OTA) software updater for embedded Linux
devices. Mender comprises a client running at the embedded device, as well as
a server that manages deployments across many devices.

This repository contains the artifacts library, which is used by the
Mender client, command line interface, server and for build integration with the Yocto Project.

The artifacts library makes it easy to programmatically work with a Mender artifact, which
is a file that can be recognized by its `.mender` suffix. Mender artifacts
can contain binaries, metadata, checksums, signatures and scripts that are
used during a deployment. The artifact format acts as a wrapper, and
uses the `tar` format to bundle several files into one.

In its simplest form, an artifact contains just a rootfs image,
along with its checksum, id and device type compatibility.


The artifacts library might also be useful for other updaters or
purposes. We are always happy to see other uses of it!


![Mender logo](https://mender.io/user/pages/04.resources/logos/logoS.png)


## Getting started

To start using Mender, we recommend that you begin with the Getting started
section in [the Mender documentation](https://docs.mender.io/).


## Using the library

You can use the reader and the writer in go in the standard way:

```
import (
        "github.com/mendersoftware/mender-artifact/areader"
        "github.com/mendersoftware/mender-artifact/awriter"
...
)
```

For sample usage, please see the [Mender client source code](https://github.com/mendersoftware/mender).


## Downloading the binaries

You can find the latest `mender-artifact` binaries in the [Downloads page on
Mender Docs](https://docs.mender.io/downloads).

## Building from source

**Note: The build process is only supported on Linux-based environments.**

### Containerized

The preferred way to build `mender-artifact` from source is containerized in `docker`.
This process can be used to generate binaries suitable for running on Linux, Windows and MacOS
hosts. 

You need the following prerequisites:
- docker [Installation instructions](https://docs.docker.com/get-docker/)
- the `make` tool. Most distributions offer a `make` package. Install it using your native
  package management tool, such as `apt`, `yum`, `pacman`, ...

In the source directory, build `mender-artifact` by issueing
```
make build-natives-contained
```

This results in the self-contained binary `mender-artifact`. You can leave it in place,
or move it to a location on your systems `PATH`.

**Note: containerized building on non-x86 platform is not supported yet**

### Native

To build `mender-artifact` from source you need the following prerequisites:
(packages given for Debian-based environments)
- the Go programming language. [Installation instructions](https://go.dev/doc/install)
- the packages `build-essential liblzma-dev libssl-dev pkg-config`

In the source directory, `mender-artifact` is built by simply issueing
```
make
```

This results in the self-contained binary `mender-artifact`. You can leave it in place,
or move it to a location on your systems `PATH`.

## Enabling auto-completion in Bash & ZSH

### Automatic installation through the Makefile

This is the easiest approach, and all that is needed it to run:

```bash
sudo make install-autocomplete-scripts
```

And the `Bash` auto-complete script will be installed to
`/etc/bash_completion.d`, and if `Zsh` is installed on the system, the
corresponding auto-completion script is installed into
`/usr/local/share/zsh/site-functions`.

You can override both the `DESTDIR` (default: empty) and `PREFIX` (default: `/usr/local`) variables to alter the destination paths, i.e:

```bash
sudo make DESTDIR=/tmp PREFIX=/usr install-autocomplete-scripts
```

### Manual installation

 auto-completion of `mender-artifact` sub-commands can be added to either ZSH or
 Bash through:

#### Bash

 The simplest way of enabling auto-completion in Bash is to copy the
 `./autocomplete/bash_autocomplete` file into `/etc/bash_completion.d/` like so:

 ```bash
sudo cp path/to/mender-aritfact/autocomplete/bash_autocomplete /etc/bash_completion.d/mender-artifact
source /etc/bash_completion.d/mender-artifact
 ```

 Alternatively the following can be added to `.bashrc`:

 ```bash
PROG=mender-artifact
source path/to/mender-artifact/autocomplete/bash_autocomplete
 ```

 #### ZSH

Auto-completion for ZSH is supported through the `zsh_autocompletion` script
found in the `./autocomplete` directory. In order to enable it consistently, add
these lines to your `.zshrc` file:

```bash
source  path/to/mender-artifact/autocomplete/zsh_autocomplete
```


## Contributing

We welcome and ask for your contribution. If you would like to contribute to Mender, please read our guide on how to best get started [contributing code or
documentation](https://github.com/mendersoftware/mender/blob/master/CONTRIBUTING.md).

## License

Mender is licensed under the Apache License, Version 2.0. See
[LICENSE](https://github.com/mendersoftware/artifacts/blob/master/LICENSE) for the
full license text.

## Security disclosure

We take security very seriously. If you come across any issue regarding
security, please disclose the information by sending an email to
[security@mender.io](security@mender.io). Please do not create a new public
issue. We thank you in advance for your cooperation.

## Connect with us

* Join the [Mender Hub discussion forum](https://hub.mender.io)
* Follow us on [Twitter](https://twitter.com/mender_io). Please
  feel free to tweet us questions.
* Fork us on [Github](https://github.com/mendersoftware)
* Create an issue in the [bugtracker](https://northerntech.atlassian.net/projects/MEN)
* Email us at [contact@mender.io](mailto:contact@mender.io)
* Connect to the [#mender IRC channel on Libera](https://web.libera.chat/?#mender)
