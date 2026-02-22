import type {ReactNode} from 'react';
import clsx from 'clsx';
import Heading from '@theme/Heading';
import styles from './styles.module.css';

type FeatureItem = {
  title: string;
  badge: string;
  description: ReactNode;
};

const FeatureList: FeatureItem[] = [
  {
    title: 'Auth-first workflows',
    badge: 'AUTH',
    description: (
      <>
        `--wtoken`, `--wrtoken`, and cookie fallbacks are documented with real
        request paths and token-rotation behavior.
      </>
    ),
  },
  {
    title: 'Cart and checkout preview',
    badge: 'CART',
    description: (
      <>
        Safe endpoint coverage for basket add/remove/clear and checkout totals
        preview, without order placement.
      </>
    ),
  },
  {
    title: 'Address-book and map validation',
    badge: 'ADDR',
    description: (
      <>
        Profile address CRUD plus `profile addresses links` for direct Google
        Maps validation of address and entrance details.
      </>
    ),
  },
];

function Feature({title, badge, description}: FeatureItem) {
  return (
    <div className={clsx('col col--4', styles.cardWrap)}>
      <div className={styles.card}>
        <span className={styles.badge}>{badge}</span>
        <Heading as="h3" className={styles.title}>{title}</Heading>
        <p className={styles.description}>{description}</p>
      </div>
    </div>
  );
}

export default function HomepageFeatures(): ReactNode {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className={styles.topLine}>
          <Heading as="h2">Built for practical terminal workflows</Heading>
          <p>
            Reference-focused docs for the real command surface of the
            unofficial community `wolt-cli` tool.
          </p>
        </div>
        <div className="row">
          {FeatureList.map((props, idx) => (
            <Feature key={idx} {...props} />
          ))}
        </div>
      </div>
    </section>
  );
}
