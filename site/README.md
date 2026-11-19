# Surf website

Static Astro site published at [https://seg6.space/surf/](https://seg6.space/surf/)
together with the Cydia/Sileo apt metadata.

Documentation Markdown is authored under `../docs` and copied into the content
collection at build time.

The site is written for iOS 6 Mobile Safari and newer. Every layout has a
`display:table` / float baseline, and CSS grid and flexbox are layered on top
inside `@supports` blocks that iOS 6 skips. Gradients carry `-webkit-` forms.
There are no custom properties, no `clip-path`, no `object-fit`, and the only
script is a small inline one that enables the Copy button. Source Sans 3 loads
from Google Fonts and falls back to Helvetica Neue and Arial.

```sh
make site-build
SITE_DIST=site/dist packaging/repository/build.sh path/to/client.deb /tmp/surf-repo
packaging/repository/verify.sh path/to/client.deb /tmp/surf-repo
```

`SITE_DIST` is required. The old `index.html.in` landing page is gone.
