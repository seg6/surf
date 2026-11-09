import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const repoRoot = path.resolve(root, '..');
const repoDocs = path.join(repoRoot, 'docs');
const outDir = path.join(root, 'src/content/docs');
const publicDir = path.join(root, 'public');

function walk(dir, files = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === 'media') continue;
      walk(full, files);
    } else if (entry.name.endsWith('.md')) {
      files.push(full);
    }
  }
  return files;
}

function titleFromMarkdown(text, fallback) {
  const match = text.match(/^#\s+(.+)$/m);
  return match ? match[1].trim() : fallback;
}

function sectionOf(rel) {
  const top = rel.split(path.sep)[0];
  if (['guide', 'reference', 'concepts', 'clients', 'develop', 'architecture'].includes(top)) {
    return top;
  }
  return 'root';
}

function rewriteLinks(markdown, slug) {
  const dir = path.posix.dirname(slug);
  return markdown.replace(/\]\(([^)]+)\)/g, (full, target) => {
    if (/^(https?:|mailto:|#|\/)/i.test(target)) return full;
    const [filePart, hash = ''] = target.split('#');
    const hashSuffix = hash ? `#${hash}` : '';
    if (!filePart.endsWith('.md')) return full;

    // Resolve as if the markdown lives at docs/<slug>.md
    const fromRepo = path.posix.normalize(`docs/${dir}/${filePart}`);
    if (!fromRepo.startsWith('docs/')) {
      const repoPath = fromRepo.replace(/\.md$/, '');
      return `](https://github.com/seg6/surf/blob/main/${repoPath}${hashSuffix})`;
    }

    let clean = fromRepo.slice('docs/'.length).replace(/\.md$/, '');
    clean = clean.replace(/^\/+/, '').replace(/\/+$/, '');
    return `](/surf/docs/${clean}/${hashSuffix})`;
  });
}

fs.rmSync(outDir, { recursive: true, force: true });
fs.mkdirSync(outDir, { recursive: true });
fs.mkdirSync(publicDir, { recursive: true });

const iconSrc = path.join(repoRoot, 'client/ios/Resources/icon-144.png');
if (fs.existsSync(iconSrc)) {
  fs.copyFileSync(iconSrc, path.join(publicDir, 'CydiaIcon.png'));
}

const files = walk(repoDocs);
const kept = [];
for (const file of files) {
  const rel = path.relative(repoDocs, file);
  const raw = fs.readFileSync(file, 'utf8');
  const slug = rel.replace(/\.md$/, '').split(path.sep).join('/');
  if (
    slug === 'index' ||
    sectionOf(rel) === 'root' ||
    /^Moved to /m.test(raw) ||
    /^Moved into /m.test(raw) ||
    raw.includes('This page moved into') ||
    raw.includes('Superseded by:') ||
    raw.includes('now live under') ||
    raw.includes('now live in')
  ) {
    continue;
  }
  const title = titleFromMarkdown(raw, slug);
  const body = rewriteLinks(raw.replace(/^---\n[\s\S]*?\n---\n/, ''), slug);
  // No `slug` frontmatter: Astro's glob loader would use it as the entry id and
  // emit duplicate-id warnings when content is rewritten under a running dev server.
  const out = `---\ntitle: ${JSON.stringify(title)}\nsection: ${JSON.stringify(sectionOf(rel))}\n---\n\n${body}`;
  const dest = path.join(outDir, `${slug}.md`);
  fs.mkdirSync(path.dirname(dest), { recursive: true });
  fs.writeFileSync(dest, out);
  kept.push(slug);
}

fs.writeFileSync(path.join(outDir, '_manifest.json'), JSON.stringify(kept, null, 2));
console.log(`Prepared ${kept.length} docs into src/content/docs`);
