[![Build Status](https://travis-ci.org/mendersoftware/mender-artifact.svg?branch=master)](https://travis-ci.org/mendersoftware/mender-artifact)
[![codecov](https://codecov.io/gh/mendersoftware/mender-artifact/branch/master/graph/badge.svg)](https://codecov.io/gh/mendersoftware/mender-artifact)
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


![Mender logo](https://mender.io/user/pages/resources/06.digital-assets/mender.io.png)


## Getting started

To start using Mender, we recommend that you begin with the Getting started
section in [the Mender documentation](https://docs.mender.io/).


## Using the library

You can use the parser and reader in go in the standard way:

```
import (
        "github.com/mendersoftware/mender-artifact/parser"
        "github.com/mendersoftware/mender-artifact/reader"
...
)
```

For sample usage, please see the [Mender client source code](https://github.com/mendersoftware/mender).

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

* Join our [Google
  group](https://groups.google.com/a/lists.mender.io/forum/#!forum/mender)
* Follow us on [Twitter](https://twitter.com/mender_io?target=_blank). Please
  feel free to tweet us questions.
* Fork us on [Github](https://github.com/mendersoftware)
* Email us at [contact@mender.io](mailto:contact@mender.io)
