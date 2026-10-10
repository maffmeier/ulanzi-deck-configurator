# AUR package `ulanzi-deck-bin`

`PKGBUILD.in` is the template; `update.sh VERSION` fills in the version and
checksums of a published GitHub release and generates `.SRCINFO`.

Publishing (needs an AUR account with SSH key):

```bash
sh packaging/aur/update.sh 0.4.0
git clone ssh://aur@aur.archlinux.org/ulanzi-deck-bin.git /tmp/aur-ulanzi-deck-bin
cp packaging/aur/{PKGBUILD,.SRCINFO,ulanzi-deck-bin.install} /tmp/aur-ulanzi-deck-bin/
cd /tmp/aur-ulanzi-deck-bin && git add -A && git commit -m "Update to 0.4.0" && git push
```
