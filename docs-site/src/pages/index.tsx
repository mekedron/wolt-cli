import type {ReactNode} from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import useBaseUrl from '@docusaurus/useBaseUrl';
import Layout from '@theme/Layout';
import HomepageFeatures from '@site/src/components/HomepageFeatures';
import Heading from '@theme/Heading';

import styles from './index.module.css';

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  const heroLogoSrc = useBaseUrl('/img/logo.png');
  return (
    <header className={clsx(styles.heroBanner)}>
      <div className={clsx('container', styles.heroGrid)}>
        <div className={styles.heroLeft}>
          <p className={styles.kicker}>Unofficial community CLI tool</p>
          <div className={styles.brandLockup}>
            <img
              className={styles.heroLogo}
              src={heroLogoSrc}
              alt="wolt-cli logo"
            />
            <Heading as="h1" className={styles.heroTitle}>
              {siteConfig.title}
            </Heading>
          </div>
          <p className={styles.heroSubtitle}>{siteConfig.tagline}</p>
          <div className={styles.buttons}>
            <Link
              className="button button--lg button--primary"
              to="/docs/cli-installation">
              Start with installation
            </Link>
            <Link className="button button--lg button--secondary" to="/docs/cli-cart-checkout">
              Cart and checkout docs
            </Link>
            <Link className="button button--lg button--secondary" to="/docs/cli-auth">
              Auth and profile docs
            </Link>
          </div>
          <p className={styles.notice}>
            Community-maintained tool documentation. This is not an official
            Wolt product or partner integration. Use the tool and credentials
            at your own risk.
          </p>
        </div>
        <div className={styles.heroRight}>
          <div className={styles.shellCard}>
            <p className={styles.shellTitle}>First command</p>
            <code>wolt configure --profile-name default --wtoken "&lt;token&gt;"</code>
          </div>
          <div className={styles.shellCardMuted}>
            <p>Safe boundary</p>
            <strong>Preview only. Final checkout uses your Wolt-saved address.</strong>
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
      title={`${siteConfig.title}`}
      description="Unofficial community CLI tool documentation for wolt-cli.">
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
