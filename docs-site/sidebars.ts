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
      label: 'Getting Started',
      collapsed: false,
      items: ['cli-overview', 'cli-installation'],
    },
    {
      type: 'category',
      label: 'Authentication',
      items: ['cli-auth'],
    },
    {
      type: 'category',
      label: 'Discovery and Menus',
      items: ['cli-discovery-search', 'cli-venue-item'],
    },
    {
      type: 'category',
      label: 'Cart and Checkout',
      items: ['cli-cart-checkout'],
    },
    {
      type: 'category',
      label: 'Profile and Orders',
      collapsed: false,
      items: ['cli-orders-profile', 'cli-profile-addresses'],
    },
    {
      type: 'category',
      label: 'Output Reference',
      items: ['cli-output-contract'],
    },
  ],
};

export default sidebars;
