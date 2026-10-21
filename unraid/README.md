# Running pascha on Unraid

`pascha.xml` is an Unraid Docker template for the image published to
`ghcr.io/saidtezel/pascha` by `.github/workflows/image.yml`.

## 1. Let Unraid pull the image

The GHCR package is private unless you change it. Either:

- **Make the package public** (simplest; the source is already public
  and the image holds no secrets): GitHub, your profile, Packages,
  `pascha`, Package settings, Change visibility. Then skip to step 2.
- **Keep it private** and log Unraid in with a classic personal access
  token that has only the `read:packages` scope. From the Unraid
  terminal:

  ```bash
  echo <TOKEN> | docker login ghcr.io -u saidtezel --password-stdin
  cp /root/.docker/config.json /boot/config/ghcr-docker-config.json
  ```

  Unraid's root filesystem lives in RAM, so the login is lost on
  reboot. Restore it at boot by adding these lines to
  `/boot/config/go`:

  ```bash
  mkdir -p /root/.docker
  cp /boot/config/ghcr-docker-config.json /root/.docker/config.json
  ```

  The token is stored in plain text on the flash drive.

## 2. Add the template

From the Unraid terminal:

```bash
wget -O /boot/config/plugins/dockerMan/templates-user/my-pascha.xml \
  https://raw.githubusercontent.com/saidtezel/pascha/main/unraid/pascha.xml
```

Then Docker tab, Add Container, pick `pascha` from the Template
drop-down, fill in the Famly and Immich fields, and Apply. The
container runs one fetch and stops; check its log for the result.

For the first run, set Post Arguments to `fetch --max-pages 0` to
backfill the whole feed, then set it back to `fetch`.

## 3. Schedule it

pascha exits after each fetch, so re-run it on a schedule with the
User Scripts plugin (Community Applications, "User Scripts"):

1. Settings, User Scripts, Add New Script, name it `pascha`.
2. Edit the script:

   ```bash
   #!/bin/bash
   docker start -a pascha
   ```

3. Set the schedule to Custom, e.g. `0 */6 * * *` for every six hours.

`docker start` re-runs the container with the settings from the
template, and `-a` puts the fetch output in the User Scripts log.

## Updating

The template tracks the `:main` tag, which is rebuilt on every push to
main. Unraid's Docker tab shows when a newer image is available; use
"update ready" / Force Update to pull it. To follow releases only,
change Repository to `ghcr.io/saidtezel/pascha:latest` once a `v*` tag
has been pushed.
