import { defineConfig } from 'astro/config';

export default defineConfig({
  site: 'https://seg6.space',
  base: '/surf/',
  // Static hosts and typed URLs often omit the slash; 'always' 404s those in preview.
  trailingSlash: 'ignore',
  image: {
    service: {
      entrypoint: 'astro/assets/services/noop',
    },
  },
  build: {
    format: 'directory',
  },
  markdown: {
    syntaxHighlight: false,
  },
});
