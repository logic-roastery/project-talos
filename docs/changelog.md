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

<div v-for="release in releases.releases" :key="release.tag_name" style="margin-bottom: 3rem;">

## [{{ release.tag_name }}]({{ release.html_url }}) — {{ formatDate(release.published_at) }}

<div v-if="release.prerelease" style="display: inline-block; padding: 2px 8px; border-radius: 4px; background: var(--vp-custom-block-tip-bg); color: var(--vp-custom-block-tip-text); font-size: 0.85em; margin-bottom: 1rem;">Pre-release</div>

<div v-html="release.body" class="release-body"></div>

</div>

<div v-if="releases.releases.length === 0" style="text-align: center; padding: 3rem; color: var(--vp-c-text-3);">

No releases found. Check [GitHub Releases](https://github.com/logic-roastery/project-talos/releases) directly.

</div>

<style>
.release-body {
  line-height: 1.7;
}
.release-body h2,
.release-body h3 {
  margin-top: 1rem;
}
.release-body ul {
  padding-left: 1.5rem;
}
.release-body li {
  margin-bottom: 0.25rem;
}
.release-body code {
  background: var(--vp-c-bg-soft);
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 0.9em;
}
.release-body a {
  color: var(--vp-c-brand-1);
}
</style>
