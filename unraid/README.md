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
drop-down, fill in the Famly and Immich fields, and Apply.

The container keeps running: it fetches once at start, then again
every **Fetch interval** (`PASCHA_INTERVAL`, default `6h`, minimum
`15m`), measured from the start of the previous fetch. A failed fetch
(Famly or Immich unreachable, an upload rejected) is logged and
retried at the next interval; open the container's log to see each
run's result and the time of the next one. Keep the interval set: the
template restarts the container whenever it exits, so without one it
would fetch back to back.

For the first run, set Post Arguments to `fetch --max-pages 0` to
backfill the whole feed, then set it back to `fetch` so each interval
only reads the newest pages.

## Updating

The template tracks the `:main` tag, which is rebuilt on every push to
main. Unraid's Docker tab shows when a newer image is available; use
"update ready" / Force Update to pull it. To follow releases only,
change Repository to `ghcr.io/saidtezel/pascha:latest` once a `v*` tag
has been pushed.
