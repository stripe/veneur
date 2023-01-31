Generate these files by running:
```
openssl req -newkey rsa:2048 -new -nodes -x509 -days 1 -out test/cert.crt -keyout test/key.key -subj "/C=US/ST=California/L=South San Francisco/O=Stripe/OU=Veneur/CN=localhost"
```