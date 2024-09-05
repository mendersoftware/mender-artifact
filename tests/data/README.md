# tests data files

## ECDSA256 Nitro HSM case

### generate the keys in the HSM

```sh
pkcs11-tool --module /usr/lib/arm-linux-gnueabihf/opensc-pkcs11.so -l --pin xx --keypairgen --key-type EC:prime256v1 --id xx
```

save the public key:

```sh
pkcs11-tool  --read-object --type pubkey --id xx -o pub.key
```

save in PEM format:

```sh
openssl pkey -pubin -in pub.key -outform PEM -out ec.pem
```

### generate artifact and sign

configure OpenSSL, add to the config:

```
[openssl_init]
engines=engine_section

[engine_section]
pkcs11 = pkcs11_section

[pkcs11_section]
engine_id = pkcs11
MODULE_PATH = /usr/lib/x86_64-linux-gnu/pkcs11/opensc-pkcs11.so
init = 0
```

get the PKCS#11 URI:

```sh
pkcs11-tool --login --pin xx --list-objects
p11tool --login --set-pin xx --list-all-privkeys 'URL you got from above'
```

sign:

```sh
mender-artifact sign --key-pkcs11 "${key}" /tmp/a0.mender -o /tmp/a0-signed-nitro.mender
# where key is the PKCS#11 URI to your private key
```

