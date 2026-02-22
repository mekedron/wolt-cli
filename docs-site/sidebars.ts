import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

/**
 * Creating a sidebar enables you to:
 - create an ordered group of docs
 - render a sidebar for each doc of that group
 - provide next/previous navigation

 The sidebars can be generated from the filesystem, or explicitly defined here.

 Create as many sidebars as you want.
 */
const sidebars: SidebarsConfig = {
  tutorialSidebar: [
    {
      type: 'category',
      label: 'Start Here',
      collapsed: false,
      items: ['cli-installation', 'cli-overview', 'cli-auth'],
    },
    {
      type: 'category',
      label: 'Discovery and Menu',
      collapsed: false,
      items: ['cli-discovery-search', 'cli-venue-item'],
    },
    {
      type: 'category',
      label: 'Cart and Checkout',
      collapsed: false,
      items: ['cli-cart-checkout'],
    },
    {
      type: 'category',
      label: 'Profile and Account',
      collapsed: false,
      items: ['cli-orders-profile', 'cli-profile-addresses'],
    },
    {
      type: 'category',
      label: 'Reference',
      collapsed: false,
      items: ['cli-output-contract'],
    },
  ],
};

export default sidebars;
