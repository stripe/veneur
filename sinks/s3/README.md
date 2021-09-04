S3 Plugin
===========

The S3 plugin archives every flush to S3 as a separate S3 object.

This plugin is still in an experimental state.



# Config Options to connect to S3

Mandatory parameters below.

* aws_s3_bucket: `string`
* aws_region: `string`

Optional parameters below.

* aws_access_key_id `string`
* aws_secret_access_key `string`

The Go AWS SDK will load up Credentials in the following order. https://docs.aws.amazon.com/sdk-for-go/api/aws/session/

1. Environment Variables `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`, `AWS_PROFILE`, `AWS_REGION`
2. Shared Credentials file `~/.aws/credentials`
3. Shared Configuration file (if SharedConfig is enabled) `export AWS_SDK_LOAD_CONFIG=1`
4. EC2 Instance Metadata (credentials only).
