import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Talos',
  description: 'Self-hosted deployment platform for Dockerized applications',
  base: '/project-talos/',

  head: [
    ['link', { rel: 'preconnect', href: 'https://fonts.googleapis.com' }],
    ['link', { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' }],
    ['link', { href: 'https://fonts.googleapis.com/css2?family=Rubik:wght@400;500;600;700&display=swap', rel: 'stylesheet' }],
  ],

  themeConfig: {
    siteTitle: 'Talos',

    nav: [
      { text: 'Guide', link: '/guide/', activeMatch: '/guide/' },
      { text: 'Architecture', link: '/architecture/', activeMatch: '/architecture/' },
      { text: 'Features', link: '/features/', activeMatch: '/features/' },
      {
        text: 'v1.0',
        items: [
          { text: 'Changelog', link: '/changelog' },
          { text: 'Contributing', link: '/contributing' },
        ],
      },
    ],

    sidebar: {
      '/guide/': [
        {
          text: 'Getting Started',
          items: [
            { text: 'Introduction', link: '/guide/' },
            { text: 'Installation', link: '/guide/installation' },
            { text: 'Configuration', link: '/guide/configuration' },
            { text: 'Your First Deploy', link: '/guide/first-deploy' },
          ],
        },
        {
          text: 'Operations',
          items: [
            { text: 'Backup & Restore', link: '/guide/backup' },
            { text: 'Upgrading', link: '/guide/upgrading' },
            { text: 'Uninstalling', link: '/guide/uninstalling' },
          ],
        },
      ],
      '/architecture/': [
        {
          text: 'Architecture',
          items: [
            { text: 'Overview', link: '/architecture/' },
            { text: 'System Components', link: '/architecture/components' },
            { text: 'Deployment Flow', link: '/architecture/deployment-flow' },
            { text: 'Data Model', link: '/architecture/data-model' },
          ],
        },
      ],
      '/features/': [
        {
          text: 'Features',
          items: [
            { text: 'Overview', link: '/features/' },
            { text: 'App Management', link: '/features/app-management' },
            { text: 'Backup System', link: '/features/backup' },
            { text: 'Managed Services', link: '/features/managed-services' },
            { text: 'Traefik Routing', link: '/features/routing' },
          ],
        },
      ],
    },

    socialLinks: [
      { icon: 'github', link: 'https://github.com/logic-roastery/project-talos' },
    ],

    search: {
      provider: 'local',
    },

    editLink: {
      pattern: 'https://github.com/logic-roastery/project-talos/edit/master/docs/:path',
      text: 'Edit this page on GitHub',
    },

    footer: {
      message: 'Released under the Apache License 2.0.',
      copyright: 'Built by Logic Roastery',
    },
  },
})
