module github.com/mendersoftware/mender-artifact

go 1.14

require (
	cloud.google.com/go/kms v1.3.0
	github.com/googleapis/gax-go/v2 v2.1.1
	github.com/hashicorp/vault/api v1.2.0
	github.com/klauspost/compress v1.13.6 // indirect
	github.com/klauspost/pgzip v1.2.5
	github.com/mendersoftware/openssl v0.0.0-20220610125625-9fe59ddd6ba4
	github.com/mendersoftware/progressbar v0.0.3
	github.com/minio/sha256-simd v1.0.0
	github.com/pkg/errors v0.9.1
	github.com/remyoudompheng/go-liblzma v0.0.0-20190506200333-81bf2d431b96
	github.com/sirupsen/logrus v1.9.0
	github.com/stretchr/testify v1.8.0
	github.com/urfave/cli v1.22.10
	golang.org/x/sys v0.0.0-20220715151400-c0bba94af5f8
	google.golang.org/genproto v0.0.0-20220207164111-0872dc986b00
	google.golang.org/protobuf v1.27.1
)
