import type {ReactNode} from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import HomepageFeatures from '@site/src/components/HomepageFeatures';
import Heading from '@theme/Heading';

import styles from './index.module.css';

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={clsx(styles.heroBanner)}>
      <div className={clsx('container', styles.heroGrid)}>
        <div className={styles.heroLeft}>
          <p className={styles.kicker}>Independent docs project</p>
          <Heading as="h1" className={styles.heroTitle}>
            {siteConfig.title}
          </Heading>
          <p className={styles.heroSubtitle}>{siteConfig.tagline}</p>
          <div className={styles.buttons}>
            <Link
              className="button button--lg button--primary"
              to="/docs/cli-overview">
              Start with CLI overview
            </Link>
            <Link className="button button--lg button--secondary" to="/docs/cli-cart-checkout">
              Cart and checkout docs
            </Link>
            <Link className="button button--lg button--secondary" to="/docs/cli-profile-addresses">
              Profile addresses docs
            </Link>
          </div>
          <p className={styles.notice}>
            Community-maintained documentation. Not an official Wolt product or
            partner integration. Use the tool and credentials at your own risk.
          </p>
        </div>
        <div className={styles.heroRight}>
          <div className={styles.shellCard}>
            <p className={styles.shellTitle}>Quick command</p>
            <code>go run ./cmd/wolt checkout preview --verbose</code>
          </div>
          <div className={styles.shellCardMuted}>
            <p>Safe boundary</p>
            <strong>Preview only. No order placement command.</strong>
          </div>
        </div>
      </div>
    </header>
  );
}

export default function Home(): ReactNode {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout
      title={`${siteConfig.title} documentation`}
      description="Unofficial community documentation for the wolt-cli tool.">
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
