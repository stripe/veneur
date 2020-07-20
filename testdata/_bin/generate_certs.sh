#!/bin/bash
set -eu -o pipefail

# This assumes openssl on macOS Catalina, which is gross and outdated,
# but what can you do...

TESTDATADIR="$(cd "$(dirname "$0")/.." || exit; pwd)"

CADIR=$(mktemp -d)
cd "$CADIR"
export OPENSSL_CONF="./openssl.cnf"

for dir in ca ca_wrong ; do
    mkdir $dir
    touch $dir/index.txt
    echo 1000 > $dir/serial
    echo 1000 > $dir/crlnumber
    mkdir -p $dir/certs $dir/crl $dir/newcerts
done
cat >openssl.cnf <<-EOF
[req]
distinguished_name = req_distinguished_name
# SHA-1 is deprecated, so use SHA-2 instead.
default_md          = sha256
string_mask         = utf8only
default_bits        = 2048

[req_distinguished_name]
# empty.

[ server_certs ]
# Extensions for server certificates (man x509v3_config).
basicConstraints = CA:FALSE
nsCertType = server
nsComment = "OpenSSL Generated Server Certificate"
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer:always
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = IP:127.0.0.1

[ client_certs ]
# Extensions for client certificates (man x509v3_config).
basicConstraints = CA:FALSE
nsCertType = client
nsComment = "OpenSSL Generated Client Certificate"
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer:always
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth

[ crl_ext ]
# empty.

[ ca ]
default_ca = CA_default

[ CA_default ]
# Directory and file locations.
dir               = ./ca
certs             = \$dir/certs
crl_dir           = \$dir/crl
new_certs_dir     = \$dir/newcerts
database          = \$dir/index.txt
serial            = \$dir/serial
certificate       = \$dir/ca.crt
private_key       = \$dir/ca.key
crlnumber         = \$dir/crlnumber
crl               = \$dir/ca.crl
crl_extensions    = crl_ext
default_crl_days  = 375
default_days      = 375
default_md        = sha256
name_opt          = ca_default
cert_opt          = ca_default
preserve          = no
policy            = policy_strict
email_in_dn       = no

[ CA_wrong ]
# Directory and file locations.
dir               = ./ca_wrong
certs             = \$dir/certs
crl_dir           = \$dir/crl
new_certs_dir     = \$dir/newcerts
database          = \$dir/index.txt
serial            = \$dir/serial
certificate       = \$dir/ca.crt
private_key       = \$dir/ca.key
crlnumber         = \$dir/crlnumber
crl               = \$dir/ca.crl
crl_extensions    = crl_ext
default_crl_days  = 375
default_days      = 375
default_md        = sha256
name_opt          = ca_default
cert_opt          = ca_default
preserve          = no
policy            = policy_strict
email_in_dn       = no

[ policy_strict ]
# The root CA should only sign intermediate certificates that match.
# See the POLICY FORMAT section of man ca.
organizationName        = match
organizationalUnitName  = optional
commonName              = supplied
emailAddress            = optional

[ v3_ca ]
# Extensions for a typical CA (man x509v3_config).
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always,issuer
basicConstraints = critical, CA:true, pathlen:0
keyUsage = critical, digitalSignature, cRLSign, keyCertSign

# -*- conf -*-
EOF

CA_SUBJ="/O=testing.veneur.org/OU=ca/CN=veneur testing CA"
SUBJ="/O=testing.veneur.org/OU=ca/CN=veneur testing server"
PASS="this is only for tests"
VALIDITY=3650 # 10 years

pwd

generate_unencrypted_key() {
    out="$1"
    echo "$PASS" | openssl genrsa -passout stdin -out "$out".encrypted
    openssl rsa -in "$out".encrypted -out "$out"
    rm "$out".encrypted
}

generate_cert() {
    key="$1"
    cert="$2"
    purpose="$3"
    ca="$4"
    name="$5"

    generate_unencrypted_key "$key"
    openssl req -new -batch -subj "${SUBJ} $name" \
            -days "${VALIDITY}" \
            -key "$key" \
            -out "$cert".csr
    openssl ca -batch -name "$ca" -utf8 -subj "${SUBJ} $name" -extensions "$purpose" -extensions SAN \
            -days "${VALIDITY}" \
            -extfile <(cat "$OPENSSL_CONF" ; echo -e "\n\n[SAN]\nsubjectAltName = DNS:localhost, IP:127.0.0.1") \
            -key "${PASS}" -in "$cert".csr -out "$cert"
    rm "$cert".csr
}

# Generate the "correct" CA cert:
echo "$PASS" | openssl genrsa -passout stdin -out ca/ca.key
openssl req -subj "${CA_SUBJ}" -batch -new -key ca/ca.key -out ca/ca.csr  -passin pass:"$PASS"
openssl ca -batch -selfsign -key "${PASS}" -in ca/ca.csr -out ca/ca.crt
rm ca/ca.csr
cp ca/ca.crt "$TESTDATADIR"/cacert.pem

# Generate the "wrong" CA cert:
echo "$PASS" | openssl genrsa -passout stdin -out ca_wrong/ca.key
openssl req -subj "${CA_SUBJ}" -batch -new -key ca_wrong/ca.key -out ca_wrong/ca.csr  -passin pass:"$PASS"
openssl ca -batch -selfsign -key "${PASS}" -name "CA_wrong" -in ca_wrong/ca.csr -out ca_wrong/ca.crt
rm ca_wrong/ca.csr

# Generate a server key & cert:
generate_cert "$TESTDATADIR"/serverkey.pem "$TESTDATADIR"/servercert.pem server_certs CA_default "server"

# Generate a client key & cert, signed by the right authority:
generate_cert "$TESTDATADIR"/clientkey.pem "$TESTDATADIR"/clientcert_correct.pem server_certs CA_default "client_correct"

# Generate a client key & cert, signed by the wrong authority:
generate_cert "$TESTDATADIR"/wrongkey.pem "$TESTDATADIR"/clientcert_wrong.pem server_certs CA_wrong "client_wrong"
rm -rf "$CADIR"
