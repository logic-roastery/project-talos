import { defineLoader } from 'vitepress'

export interface Release {
  tag_name: string
  name: string
  body: string
  html_url: string
  published_at: string
  prerelease: boolean
  draft: boolean
}

export interface Data {
  releases: Release[]
  latestTag: string
}

declare const data: Data
export { data }

export default defineLoader({
  async load(): Promise<Data> {
    const res = await fetch(
      'https://api.github.com/repos/logic-roastery/project-talos/releases?per_page=20'
    )

    if (!res.ok) {
      console.warn(`[releases] GitHub API returned ${res.status}`)
      return { releases: [], latestTag: 'v0.0.0' }
    }

    const releases: Release[] = await res.json()

    const filtered = releases
      .filter((r) => !r.draft)
      .sort(
        (a, b) =>
          new Date(b.published_at).getTime() -
          new Date(a.published_at).getTime()
      )

    return {
      releases: filtered,
      latestTag: filtered[0]?.tag_name ?? 'v0.0.0',
    }
  },
})
