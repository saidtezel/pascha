import { defineConfig } from 'astro/config';

export default defineConfig({
  site: 'https://pascha.dunn.dev',
  output: 'static',
  trailingSlash: 'always',
  build: {
    format: 'directory',
  },
});
