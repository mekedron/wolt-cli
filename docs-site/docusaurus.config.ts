import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

const config: Config = {
  title: 'wolt-cli',
  tagline: 'Independent CLI docs for discovery, carts, and checkout preview',
  favicon: 'img/favicon.ico',

  // Future flags, see https://docusaurus.io/docs/api/docusaurus-config#future
  future: {
    v4: true, // Improve compatibility with the upcoming Docusaurus v4
  },

  // Set the production url of your site here
  url: 'https://valaraucoo.github.io',
  // Set the /<baseUrl>/ pathname under which your site is served.
  baseUrl: '/',

  organizationName: 'Valaraucoo',
  projectName: 'wolt-cli',

  onBrokenLinks: 'throw',

  // Even if you don't use internationalization, you can use this field to set
  // useful metadata like html lang. For example, if your site is Chinese, you
  // may want to replace "en" with "zh-Hans".
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },
  markdown: {
    mermaid: true,
  },
  themes: ['@docusaurus/theme-mermaid'],

  presets: [
    [
      'classic',
      {
        docs: {
          path: '../docs',
          routeBasePath: 'docs',
          sidebarPath: './sidebars.ts',
        },
        blog: {
          showReadingTime: true,
          feedOptions: {
            type: ['rss', 'atom'],
            xslt: true,
          },
          // Please change this to your repo.
          // Remove this to remove the "edit this page" links.
          editUrl:
            'https://github.com/Valaraucoo/wolt-cli/tree/master/',
          // Useful options to enforce blogging best practices
          onInlineTags: 'warn',
          onInlineAuthors: 'warn',
          onUntruncatedBlogPosts: 'warn',
        },
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    image: 'img/docusaurus-social-card.jpg',
    colorMode: {
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'wolt-cli',
      logo: {
        alt: 'wolt-cli logo',
        src: 'img/logo.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'tutorialSidebar',
          position: 'left',
          label: 'Documentation',
        },
        {to: '/docs/cli-overview', label: 'Getting started', position: 'left'},
        {to: '/docs/cli-cart-checkout', label: 'Cart and checkout', position: 'left'},
        {to: '/docs/cli-orders-profile', label: 'Profile', position: 'left'},
        {
          href: 'https://github.com/Valaraucoo/wolt-cli',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'light',
      links: [
        {
          title: 'Documentation',
          items: [
            {
              label: 'CLI overview',
              to: '/docs/cli-overview',
            },
            {
              label: 'Auth commands',
              to: '/docs/cli-auth',
            },
            {
              label: 'Cart and checkout',
              to: '/docs/cli-cart-checkout',
            },
            {
              label: 'Profile commands',
              to: '/docs/cli-orders-profile',
            },
            {
              label: 'Address book guide',
              to: '/docs/cli-profile-addresses',
            },
          ],
        },
        {
          title: 'Project',
          items: [
            {
              label: 'Repository',
              href: 'https://github.com/Valaraucoo/wolt-cli',
            },
            {
              label: 'Security',
              href: 'https://github.com/Valaraucoo/wolt-cli/blob/master/SECURITY.md',
            },
          ],
        },
        {
          title: 'Notice',
          items: [
            {
              label: 'Unofficial and independent',
              to: '/',
            },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} wolt-cli contributors. This is an unofficial community tool, not affiliated with Wolt. Use is at your own responsibility.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
