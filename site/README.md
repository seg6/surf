# Surf website

Static Astro site published at [https://seg6.space/surf/](https://seg6.space/surf/)
together with the Cydia/Sileo apt metadata.

Documentation Markdown is authored under `../docs` and copied into the content
collection at build time.

The whole site is written for iOS 6 WebKit and newer: no flexbox, CSS grid,
custom properties, sticky positioning, client JavaScript, or `@font-face`.
Layout uses `display:table` / floats. System fonts only.

```sh
make site-build
SITE_DIST=site/dist packaging/repository/build.sh path/to/client.deb /tmp/surf-repo
packaging/repository/verify.sh path/to/client.deb /tmp/surf-repo
```

`SITE_DIST` is required. The old `index.html.in` landing page is gone.
