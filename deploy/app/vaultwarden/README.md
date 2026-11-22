# Secrets

## RSA Key

Vaultwarden uses an RSA key to do some encryption. That must be placed in a
seret named `rsa-key` in the `app` namespaces.

The key can be created using: `openssl genrsa -out rsa_key.pem 2048`. Do not
set any password on it.

Then, the value for the sealed secret can be encrypted by:
```
$ cat rsa_key.pem | kubeseal --raw --namespace app --scope namespace-wide
```

See the first RD in `secret.yaml` for this definition.

## Push notifications

Vaultwarden supports push notifications to clients through Bitwarden's servers.
This requires an ID and key from bitwarden, which will be stored in a secret.
To create that secret, you can use:
 
```yaml
apiVersion: bitnami.com/v1alpha1
kind: SealedSecret
metadata:
  name: relay
  annotations:
    sealedsecrets.bitnami.com/namespace-wide: "true"
spec:
  encryptedData:
    id: <encrypted id>
    key: <encrypted id>
```

You can encrypt the data as follows:
```
$ echo -n "<id>" | kubeseal --raw --namespace app --scope namespace-wide
```
