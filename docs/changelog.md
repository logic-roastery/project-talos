# Changelog

All notable changes to Talos are documented here. Releases are automatically pulled from [GitHub Releases](https://github.com/logic-roastery/project-talos/releases).

<script setup>
import { data as releases } from './.vitepress/loaders/releases.data.ts'

function formatDate(iso) {
  return new Date(iso).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}
</script>

<div class="releases">

<div v-for="release in releases.releases" :key="release.tag_name" class="release">

<div class="release-header">
  <a :href="release.html_url" class="release-tag" target="_blank" rel="noopener">
    {{ release.tag_name }}
  </a>
  <span v-if="release.prerelease" class="pre-release-badge">Pre-release</span>
  <span class="release-date">Published {{ formatDate(release.published_at) }}</span>
</div>

<div class="release-body" v-html="release.body"></div>

<div class="release-footer">
  <a :href="release.html_url" target="_blank" rel="noopener" class="release-link">
    View on GitHub
  </a>
</div>

<hr class="release-divider" />

</div>

</div>

<div v-if="releases.releases.length === 0" class="empty-state">

No releases found. Check [GitHub Releases](https://github.com/logic-roastery/project-talos/releases) directly.

</div>

<style>
.releases {
  margin-top: 1.5rem;
}

.release {
  margin-bottom: 0;
}

.release-header {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 1rem;
}

.release-tag {
  display: inline-flex;
  align-items: center;
  padding: 4px 12px;
  font-size: 1rem;
  font-weight: 600;
  color: var(--vp-c-brand-1);
  background: var(--vp-c-brand-soft);
  border-radius: 20px;
  text-decoration: none;
  transition: background 0.2s;
}

.release-tag:hover {
  background: var(--vp-c-brand-1);
  color: #fff;
}

.pre-release-badge {
  display: inline-block;
  padding: 2px 8px;
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--vp-c-brand-1);
  border: 1px solid var(--vp-c-brand-1);
  border-radius: 20px;
  line-height: 1.4;
}

.release-date {
  font-size: 0.875rem;
  color: var(--vp-c-text-3);
}

.release-body {
  line-height: 1.7;
  color: var(--vp-c-text-1);
}

.release-body h2 {
  font-size: 1.25rem;
  font-weight: 600;
  margin-top: 1.5rem;
  margin-bottom: 0.75rem;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid var(--vp-c-divider);
}

.release-body h3 {
  font-size: 1.1rem;
  font-weight: 600;
  margin-top: 1.25rem;
  margin-bottom: 0.5rem;
}

.release-body ul {
  padding-left: 1.5rem;
  margin: 0.5rem 0;
}

.release-body li {
  margin-bottom: 0.35rem;
  line-height: 1.6;
}

.release-body p {
  margin: 0.5rem 0;
}

.release-body code {
  background: var(--vp-c-bg-soft);
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 0.85em;
  font-family: var(--vp-font-family-mono);
}

.release-body pre {
  background: var(--vp-c-bg-soft);
  padding: 1rem;
  border-radius: 8px;
  overflow-x: auto;
}

.release-body pre code {
  background: none;
  padding: 0;
}

.release-body a {
  color: var(--vp-c-brand-1);
  text-decoration: none;
}

.release-body a:hover {
  text-decoration: underline;
}

.release-body strong {
  font-weight: 600;
}

.release-footer {
  margin-top: 1rem;
}

.release-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 0.875rem;
  color: var(--vp-c-text-2);
  text-decoration: none;
  transition: color 0.2s;
}

.release-link:hover {
  color: var(--vp-c-brand-1);
}

.release-divider {
  border: none;
  border-top: 1px solid var(--vp-c-divider);
  margin: 2rem 0;
}

.release:last-child .release-divider {
  display: none;
}

.empty-state {
  text-align: center;
  padding: 3rem;
  color: var(--vp-c-text-3);
}

/* GitHub-style contributor avatars */
.release-body img {
  border-radius: 50%;
  vertical-align: middle;
  margin-right: 4px;
}

/* GitHub-style PR links */
.release-body a[href*="github.com"] {
  color: var(--vp-c-brand-1);
}
</style>
