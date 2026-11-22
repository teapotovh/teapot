# Adding users

To add a user (role in pg) you first must create a secret with the credentials,
replace the username and password values with the base64 encoded version and
apply the following manifest:

```yaml
apiVersion: bitnami.com/v1alpha1
kind: SealedSecret
metadata:
  name: user-<user>
  annotations:
    sealedsecrets.bitnami.com/namespace-wide: "true"
spec:
  data:
    username: <user>
  encryptedData:
      password: <encrypted password>
```

To get the encrypted username/password:
```
$ echo -n <password> | kubeseal --raw --namespace storage --scope namespace-wide
```

Then, add the relative role to `cluster.yaml`.
