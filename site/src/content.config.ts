import { defineCollection, z } from 'astro:content';
import { glob } from 'astro/loaders';

const docs = defineCollection({
  // Id comes from the file path only. Do not put a `slug` field in frontmatter:
  // Astro's glob loader treats data.slug as the entry id and warns on resync.
  loader: glob({
    pattern: '**/*.md',
    base: './src/content/docs',
    generateId: ({ entry }) => entry.replace(/\.md$/, ''),
  }),
  schema: z.object({
    title: z.string(),
    section: z.string(),
  }),
});

export const collections = { docs };
