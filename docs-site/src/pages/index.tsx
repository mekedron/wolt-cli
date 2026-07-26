import {useState, type ReactNode} from 'react';
import useBaseUrl from '@docusaurus/useBaseUrl';
import Layout from '@theme/Layout';

import './landing.css';

type InstallTab = 'brew' | 'src';

const INSTALL_TABS: Array<{id: InstallTab; label: string; copy: string; code: ReactNode}> = [
  {
    id: 'brew',
    label: 'Homebrew',
    copy: 'brew install mekedron/tap/wolt-cli',
    code: (
      <>
        <span className="tk-mut"># One-liner — tap is added implicitly</span>
        {'\n'}
        <span className="tk-fn">brew</span> install mekedron/tap/wolt-cli{'\n'}
        {'\n'}
        <span className="tk-mut"># Or add the tap first, then install:</span>
        {'\n'}
        <span className="tk-fn">brew</span> tap mekedron/tap{'\n'}
        <span className="tk-fn">brew</span> install wolt-cli
      </>
    ),
  },
  {
    id: 'src',
    label: 'From source',
    copy:
      'git clone https://github.com/mekedron/wolt-cli.git\ncd wolt-cli\ngo build -o bin/wolt ./cmd/wolt\n./bin/wolt --help',
    code: (
      <>
        <span className="tk-mut"># Requires Go 1.26+</span>
        {'\n'}
        <span className="tk-fn">git</span> clone https://github.com/mekedron/wolt-cli.git{'\n'}
        <span className="tk-fn">cd</span> wolt-cli{'\n'}
        <span className="tk-fn">go</span> build -o bin/wolt ./cmd/wolt{'\n'}
        ./bin/wolt --help
      </>
    ),
  },
];

function useCopy() {
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const copy = (key: string, text: string) => {
    const finish = () => {
      setCopiedKey(key);
      window.setTimeout(() => {
        setCopiedKey((current) => (current === key ? null : current));
      }, 1400);
    };
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(text).then(finish, finish);
    } else {
      finish();
    }
  };
  return {copiedKey, copy};
}

function InlineCmd({
  id,
  text,
  size,
  children,
}: {
  id: string;
  text: string;
  size?: 'lg';
  children: ReactNode;
}) {
  const {copiedKey, copy} = useCopy();
  const isCopied = copiedKey === id;
  return (
    <div className={`cmd${size === 'lg' ? ' cmd--lg' : ''}${isCopied ? ' is-copied' : ''}`}>
      <span className="cmd__prompt">$</span>
      <span className="cmd__text">{children}</span>
      <button
        type="button"
        className="cmd__copy"
        aria-label="Copy install command"
        onClick={() => copy(id, text)}>
        <svg viewBox="0 0 24 24" width={size === 'lg' ? 18 : 16} height={size === 'lg' ? 18 : 16} aria-hidden="true">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M8 8V5a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2h-3M5 8h9a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-9a2 2 0 0 1 2-2Z"
          />
        </svg>
        <span className="cmd__copied">copied</span>
      </button>
    </div>
  );
}

function InstallTabs() {
  const [active, setActive] = useState<InstallTab>('brew');
  const {copiedKey, copy} = useCopy();
  return (
    <div className="tabs">
      <div className="tabs__bar" role="tablist">
        {INSTALL_TABS.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            className={`tabs__tab${active === tab.id ? ' is-active' : ''}`}
            aria-selected={active === tab.id}
            onClick={() => setActive(tab.id)}>
            {tab.label}
          </button>
        ))}
      </div>
      <div className="tabs__panels">
        {INSTALL_TABS.map((tab) => {
          const isCopied = copiedKey === `tab-${tab.id}`;
          return (
            <div
              key={tab.id}
              className={`tabs__panel${active === tab.id ? ' is-active' : ''}`}>
              <div className="codeblock">
                <pre>
                  <code>{tab.code}</code>
                </pre>
                <button
                  type="button"
                  className={`codeblock__copy${isCopied ? ' is-copied' : ''}`}
                  onClick={() => copy(`tab-${tab.id}`, tab.copy)}>
                  {isCopied ? 'Copied' : 'Copy'}
                </button>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function Hero() {
  return (
    <section className="hero">
      <div className="hero__grid">
        <div className="hero__copy">
          <span className="eyebrow">
            <span className="eyebrow__dot" />
            v1 · Community Go CLI · MIT
          </span>
          <h1 className="hero__title">
            Discover, browse, and cart
            <br />
            <span className="grad">without leaving the terminal.</span>
          </h1>
          <p className="hero__lede">
            <strong>wolt-cli</strong> is an unofficial, community-built Go CLI for
            interacting with Wolt endpoints. <code>wolt top</code> for the
            "what should I eat right now" view, <code>wolt feed --summary</code>{' '}
            to glance the whole home page in one screen, plus venue drilldown,
            cart, and checkout preview — straight from your shell.
          </p>

          <div className="install-row" aria-label="Quick install">
            <InlineCmd id="hero-cmd" text="brew install mekedron/tap/wolt-cli">
              <span className="tk-fn">brew</span> install mekedron/tap/wolt-cli
            </InlineCmd>
          </div>

          <div className="hero__ctas">
            <a className="btn btn--primary btn--lg" href="#install">
              Get started
            </a>
            <a className="btn btn--ghost btn--lg" href="#example">
              See real example →
            </a>
          </div>

          <ul className="meta-strip" aria-label="Project facts">
            <li>
              <span className="meta-strip__k">Lang</span>
              <span className="meta-strip__v">Go 1.26+</span>
            </li>
            <li>
              <span className="meta-strip__k">Install</span>
              <span className="meta-strip__v">Homebrew / source</span>
            </li>
            <li>
              <span className="meta-strip__k">Output</span>
              <span className="meta-strip__v">table · json · yaml</span>
            </li>
            <li>
              <span className="meta-strip__k">License</span>
              <span className="meta-strip__v">MIT</span>
            </li>
          </ul>
        </div>

        <div className="terminal" role="img" aria-label="Terminal demo of wolt-cli">
          <div className="terminal__chrome">
            <span className="dot dot--r" />
            <span className="dot dot--y" />
            <span className="dot dot--g" />
            <span className="terminal__title">~/projects — zsh — 92×24</span>
          </div>
          <pre className="terminal__body">
            <code>
              <span className="t-mut">$</span> <span className="t-fn">wolt</span> top 5{'\n'}
              <span className="t-hd">Top 5 venues</span>{'\n'}
              <span className="t-hd">Venue                        Tagline                        Rating  ETA</span>{'\n'}
              <span className="t-fl">%</span> Noodle Story Kamppi        Fresh homemade noodles         9.6    15–25 m{'\n'}
              <span className="t-fl">%</span> Putte's Bar &amp; Pizza         Artesaanipizzaa rakkaudella    9.0    20–30 m{'\n'}
              <span className="t-fl">%</span> Kotipizza Kamppi           Kuuma, kuumempi, Kotipizza     8.4    15–25 m{'\n'}
              <span className="t-fl">%</span> KFC Kamppi                 It's finger lickin' good       8.2    10–20 m{'\n'}
              <span className="t-fl">%</span> Friends &amp; Brgrs            Maailman parasta kotimaista    8.6    15–25 m{'\n'}
              {'\n'}
              <span className="t-mut">$</span> <span className="t-fn">wolt</span> feed <span className="t-fl">--summary</span>{'\n'}
              <span className="t-hd">Section                Kind    Count  Top items</span>{'\n'}
              Dinner near you        venues  6      Noodle Story · Putte's · Kotipizza · …{'\n'}
              Popular stores         brands  6      Wolt Market · K-Supermarket · K-Market{'\n'}
              Fastest delivery       venues  6      KFC Kamppi · McDonald's · Picnic · …{'\n'}
              Top-rated              venues  6      Café Bar No 9 · Hills Dumplings · …{'\n'}
              {'\n'}
              <span className="t-mut">$</span> <span className="t-fn">wolt</span> cart add noodle-story-kamppi <span className="t-fl">--query</span> "Teriyaki Udon"{'\n'}
              <span className="t-ok">✓</span> added 1× <span className="t-st">Teriyaki Udon</span>   subtotal <span className="t-st">€15.80</span>{'\n'}
              {'\n'}
              <span className="t-mut">$</span> <span className="t-fn">wolt</span> checkout{'\n'}
              {'  '}items                           <span className="t-st">€15.80</span>{'\n'}
              {'  '}delivery                         <span className="t-st">€2.90</span>{'\n'}
              {'  '}─────────────────────────{'\n'}
              {'  '}<span className="t-em">total</span>                          <span className="t-em">€18.70</span>{'\n'}
              <span className="t-mut">$</span> <span className="t-cursor">▋</span>
            </code>
          </pre>
        </div>
      </div>
    </section>
  );
}

function Trust() {
  const cells = [
    {big: '11', lbl: 'top-level commands'},
    {big: '3', lbl: 'output formats'},
    {big: '1', lbl: 'binary, zero deps'},
    {big: '0', lbl: 'orders placed by CLI'},
  ];
  return (
    <section className="trust" aria-label="At a glance">
      <div className="trust__row">
        {cells.map((c) => (
          <div key={c.lbl} className="trust__cell">
            <span className="trust__big">{c.big}</span>
            <span className="trust__lbl">{c.lbl}</span>
          </div>
        ))}
      </div>
    </section>
  );
}

function Features() {
  const items: Array<{title: ReactNode; body: ReactNode; cmd: string; icon: ReactNode}> = [
    {
      icon: (
        <svg viewBox="0 0 24 24">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M3 5h18M3 12h18M3 19h12"
          />
        </svg>
      ),
      title: <>Discovery &amp; top picks</>,
      body: (
        <>
          <code>wolt feed</code> for the section-grouped home page,{' '}
          <code>wolt feed --summary</code> for a one-line overview,{' '}
          <code>wolt top 10</code> for a single ranked list across every venue
          carousel. Brand sections render as a compact one-liner.
        </>
      ),
      cmd: 'wolt top 10',
    },
    {
      icon: (
        <svg viewBox="0 0 24 24">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M4 7h16M4 12h16M4 17h10"
          />
          <circle cx="20" cy="17" r="2" fill="currentColor" />
        </svg>
      ),
      title: <>Venue details &amp; menus</>,
      body: (
        <>
          Inspect a venue's hours, menu, categories, and item details. Add{' '}
          <code>--query</code> for assortment search, <code>--include-options</code>{' '}
          for the full option matrix. <code>venue hours</code> reads the supported
          static venue payload directly, without the retired restaurant endpoint.
        </>
      ),
      cmd: 'wolt venue menu <slug> --query "udon"',
    },
    {
      icon: (
        <svg viewBox="0 0 24 24">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M6 7h12l-1.4 9.2a2 2 0 0 1-2 1.8h-5.2a2 2 0 0 1-2-1.8L6 7Zm0 0L5 4H3"
          />
          <circle cx="10" cy="21" r="1.4" fill="currentColor" />
          <circle cx="16" cy="21" r="1.4" fill="currentColor" />
        </svg>
      ),
      title: <>Cart operations</>,
      body: (
        <>
          <code>cart</code>, <code>count</code>, <code>add</code>,{' '}
          <code>remove</code>, <code>clear</code>. Add items by item id, by Wolt
          URL, or by name (<code>--query "Teriyaki Udon"</code>). Option
          values resolve by case-insensitive name too.
        </>
      ),
      cmd: 'wolt cart add <venue> --query "<item>"',
    },
    {
      icon: (
        <svg viewBox="0 0 24 24">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M4 12h6l2-3 4 6 2-3h2"
          />
        </svg>
      ),
      title: 'Checkout preview',
      body: (
        <>
          Run <code>wolt checkout</code> to project totals, fees, and delivery
          cost from your current cart — without ever placing an order.
        </>
      ),
      cmd: 'wolt checkout --venue-id <id>',
    },
    {
      icon: (
        <svg viewBox="0 0 24 24">
          <circle cx="12" cy="8" r="4" fill="none" stroke="currentColor" strokeWidth="1.6" />
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            d="M4 20c1.5-3.5 4.7-5.5 8-5.5s6.5 2 8 5.5"
          />
        </svg>
      ),
      title: <>Account &amp; orders</>,
      body: (
        <>
          Auth status, profile, order history, addresses, payments, and
          favourites — all paginated, all read-only by default. Expired sessions
          surface a friendly "<code>wolt login</code>" hint.
        </>
      ),
      cmd: 'wolt account orders --limit 20',
    },
    {
      icon: (
        <svg viewBox="0 0 24 24">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M4 19V9m6 10V5m6 14v-7m-13 7h17"
          />
        </svg>
      ),
      title: <>Local stats dashboard</>,
      body: (
        <>
          <code>wolt stats</code> downloads a pre-built dashboard bundle from{' '}
          <a href="https://github.com/mekedron/wolt-stats" target="_blank" rel="noreferrer">
            wolt-stats
          </a>{' '}
          releases, syncs your order history into a local SQLite file, serves
          everything at <code>127.0.0.1:5173</code>, and opens the browser. No
          Node.js needed — sync is pure Go, dashboard is static HTML.
        </>
      ),
      cmd: 'wolt stats',
    },
    {
      icon: (
        <svg viewBox="0 0 24 24">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M12 3v3M12 18v3M3 12h3M18 12h3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1"
          />
          <circle cx="12" cy="12" r="4" fill="none" stroke="currentColor" strokeWidth="1.6" />
        </svg>
      ),
      title: 'Browser-driven login',
      body: (
        <>
          <code>wolt login</code> opens managed Chrome at{' '}
          <code>127.0.0.1:9222</code>, waits for you to sign in to wolt.com, and
          extracts cookies + tokens. Manual fallback via <code>--wtoken</code> /{' '}
          <code>--wrtoken</code>. Tokens auto-refresh via the saved refresh
          token.
        </>
      ),
      cmd: 'wolt login',
    },
    {
      icon: (
        <svg viewBox="0 0 24 24">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M3 5h18v3H3zM3 11h18v3H3zM3 17h12v3H3z"
          />
        </svg>
      ),
      title: 'Pipeable output',
      body: (
        <>
          Every command emits <code>table</code>, <code>json</code>, or <code>yaml</code>.
          Pipe straight into <code>jq</code>, <code>yq</code>, or your own scripts.
        </>
      ),
      cmd: "--format json | jq '.data.items[]'",
    },
    {
      icon: (
        <svg viewBox="0 0 24 24">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M12 2C7 8 4 11 4 14a8 8 0 0 0 16 0c0-3-3-6-8-12Z"
          />
        </svg>
      ),
      title: 'Location override',
      body: (
        <>
          Pass <code>--address</code> or <code>--lat/--lon</code> per command.
          Preview-only — final orders still use your saved Wolt address.
        </>
      ),
      cmd: '--address "Mannerheimintie 1, Helsinki"',
    },
    {
      icon: (
        <svg viewBox="0 0 24 24">
          <rect
            x="3"
            y="5"
            width="18"
            height="14"
            rx="2"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
          />
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M7 10l3 2-3 2M13 14h4"
          />
        </svg>
      ),
      title: <>MCP server for AI agents</>,
      body: (
        <>
          The companion <code>wolt-mcp</code> binary speaks the{' '}
          <a href="https://modelcontextprotocol.io" target="_blank" rel="noreferrer">
            Model Context Protocol
          </a>
          . Wire it into Claude Desktop, Claude Code, or Cursor and the model
          gets typed, schema-described tools for feed, search, cart, and
          checkout-preview. Auth is shared with the CLI.
        </>
      ),
      cmd: '{"mcpServers":{"wolt":{"command":"wolt-mcp"}}}',
    },
  ];

  return (
    <section id="features" className="features">
      <header className="section-head">
        <span className="section-head__eyebrow">What it covers</span>
        <h2 className="section-head__title">Every Wolt surface, in your shell.</h2>
        <p className="section-head__lede">
          From discovery to checkout preview — the same flows you'd click through in the
          app, scriptable and pipeable.
        </p>
      </header>

      <div className="features__grid">
        {items.map((it, i) => (
          <article key={i} className="feature">
            <div className="feature__icon" aria-hidden="true">
              {it.icon}
            </div>
            <h3>{it.title}</h3>
            <p>{it.body}</p>
            <code className="feature__cmd">{it.cmd}</code>
          </article>
        ))}
      </div>
    </section>
  );
}

function Stats() {
  const overview = useBaseUrl('/img/stats/dashboard-overview.png');
  return (
    <section id="stats" className="stats">
      <header className="section-head">
        <span className="section-head__eyebrow">Bring your data home</span>
        <h2 className="section-head__title">
          <code>wolt stats</code> — your order history, your dashboard.
        </h2>
        <p className="section-head__lede">
          One command syncs every order you've ever placed into a local SQLite
          database and opens a local dashboard at{' '}
          <code>http://127.0.0.1:5173</code>. Spend breakdown, top venues,
          favourite items — all derived from the same payloads the CLI already
          speaks. Nothing leaves your machine.
        </p>
      </header>

      <div className="stats__grid">
        <div className="stats__copy">
          <ul className="stats__bullets">
            <li>
              <strong>Local-only data.</strong> SQLite at{' '}
              <code>~/.wolt/stats/db/wolt-history.sqlite</code>, dashboard pinned
              to a versioned GitHub release, no telemetry. The dashboard is a
              static SvelteKit app reading the file in your browser.
            </li>
            <li>
              <strong>Incremental by default.</strong> Reruns scan until they
              hit an order they already know, so the second run takes seconds.{' '}
              <code>--resync</code> forces a full rebuild;{' '}
              <code>--no-sync</code> just re-opens the dashboard.
            </li>
            <li>
              <strong>Adaptive rate-limiting.</strong> Honors Wolt's{' '}
              <code>Retry-After</code> header and tunes per-call pacing to
              whatever rate your account's throttle window will sustain. A run
              that hits 429s settles on the right speed and finishes itself.
            </li>
            <li>
              <strong>Everything Wolt knows.</strong> Catalog and detail
              payloads land verbatim in the DB: line items, options, payments,
              delivery distance, service fees, discounts, gift cards, adjustment
              rows, creation/delivery times. The dashboard derives spend by
              month, top venues, item leaderboards, and weekday/hour patterns
              on top.
            </li>
          </ul>
          <pre className="snippet">
            <code>
              <span className="tk-fn">wolt</span> stats{'\n'}
              <span className="tk-mut"># Re-open without re-syncing</span>
              {'\n'}
              <span className="tk-fn">wolt</span> stats{' '}
              <span className="tk-fl">--no-sync</span>
              {'\n'}
              <span className="tk-mut"># Force a full re-scan of every order</span>
              {'\n'}
              <span className="tk-fn">wolt</span> stats{' '}
              <span className="tk-fl">--resync</span>
            </code>
          </pre>
          <p className="stats__more">
            <a href={useBaseUrl('/docs/stats')}>Full stats reference →</a>
          </p>
        </div>
        <div className="stats__media">
          <figure className="stats__shot">
            <img src={overview} alt="wolt stats dashboard overview" loading="lazy" />
          </figure>
        </div>
      </div>
    </section>
  );
}

function Install() {
  return (
    <section id="install" className="install">
      <header className="section-head">
        <span className="section-head__eyebrow">Install</span>
        <h2 className="section-head__title">One command. One binary.</h2>
        <p className="section-head__lede">
          Homebrew is the recommended path on macOS and Linux. Building from
          source is a single <code>go build</code>.
        </p>
      </header>

      <InstallTabs />

      <div className="install__after">
        <div className="install__step">
          <span className="install__num">1</span>
          <h4>Log in</h4>
          <p>
            Opens managed Chrome at <code>127.0.0.1:9222</code>, waits for you
            to sign in to wolt.com, and saves the cookies + tokens locally.
            Manual fallback works too.
          </p>
          <pre className="snippet">
            <code>
              <span className="tk-fn">wolt</span> login{'\n'}
              <span className="tk-fn">wolt</span> login <span className="tk-fl">--wtoken</span> "&lt;jwt&gt;" <span className="tk-fl">--wrtoken</span> "&lt;refresh&gt;"
            </code>
          </pre>
        </div>
        <div className="install__step">
          <span className="install__num">2</span>
          <h4>Verify it works</h4>
          <p>
            Status validates the saved session against{' '}
            <code>/v1/user/me</code>. Expired tokens auto-refresh; if that
            fails you get a friendly hint to re-run <code>wolt login</code>.
          </p>
          <pre className="snippet">
            <code>
              <span className="tk-fn">wolt</span> status <span className="tk-fl">--verbose</span>{'\n'}
              <span className="tk-fn">wolt</span> account <span className="tk-fl">--format</span> json
            </code>
          </pre>
        </div>
        <div className="install__step">
          <span className="install__num">3</span>
          <h4>Discover or drill in</h4>
          <p>
            One ranked list with <code>wolt top</code>, a one-line overview with{' '}
            <code>wolt feed --summary</code>, or pipe straight into{' '}
            <code>jq</code> for automation.
          </p>
          <pre className="snippet">
            <code>
              <span className="tk-fn">wolt</span> top 10{'\n'}
              <span className="tk-fn">wolt</span> feed <span className="tk-fl">--summary</span>{'\n'}
              <span className="tk-fn">wolt</span> venues <span className="tk-fl">--query</span> "ramen" <span className="tk-fl">--format</span> json
            </code>
          </pre>
        </div>
      </div>
    </section>
  );
}

function Example() {
  return (
    <section id="example" className="example">
      <header className="section-head section-head--dark">
        <span className="section-head__eyebrow">Real example</span>
        <h2 className="section-head__title">Build a custom WHOPPER meal — end to end.</h2>
        <p className="section-head__lede">
          Five steps, all in your terminal. No order is placed; checkout is preview only.
        </p>
      </header>

      <ol className="steps">
        <li className="step">
          <header>
            <span className="step__n">01</span>
            <h3>Pick a venue</h3>
          </header>
          <p>
            <code>wolt top</code> ranks venues across every curated feed
            section. <code>wolt venues --query</code> narrows by keyword. Both
            print copy-paste-ready slugs.
          </p>
          <pre className="snippet snippet--dark">
            <code>
              <span className="tk-fn">wolt</span> top 10{'\n'}
              <span className="tk-fn">wolt</span> venues <span className="tk-fl">--query</span> "burger king" <span className="tk-fl">--limit</span> 10
            </code>
          </pre>
        </li>
        <li className="step">
          <header>
            <span className="step__n">02</span>
            <h3>Inspect the menu</h3>
          </header>
          <p>
            Use <code>--query</code> for assortment search or{' '}
            <code>--include-options</code> for the full option matrix.
          </p>
          <pre className="snippet snippet--dark">
            <code>
              <span className="tk-fn">wolt</span> venue menu burger-king-finnoo <span className="tk-fl">--query</span> "whopper" <span className="tk-fl">--include-options</span>
            </code>
          </pre>
        </li>
        <li className="step">
          <header>
            <span className="step__n">03</span>
            <h3>Read the options</h3>
          </header>
          <p>
            <code>venue item</code> renders the option groups, their values, and
            min/max requirements.
          </p>
          <pre className="snippet snippet--dark">
            <code>
              <span className="tk-fn">wolt</span> venue item burger-king-finnoo &lt;item-id&gt;{'\n'}
              <span className="tk-fn">wolt</span> venue item "https://wolt.com/.../venue/burger-king-finnoo/itemid-&lt;id&gt;"
            </code>
          </pre>
        </li>
        <li className="step">
          <header>
            <span className="step__n">04</span>
            <h3>Add to cart by name</h3>
          </header>
          <p>
            Repeatable <code>--option Group=Value</code> resolves option values
            by case-insensitive name — no need to copy 24-char IDs by hand.
          </p>
          <pre className="snippet snippet--dark">
            <code>
              <span className="tk-fn">wolt</span> cart add burger-king-finnoo <span className="tk-fl">--query</span> "WHOPPER Meal" \{'\n'}
              {'  '}<span className="tk-fl">--option</span> "Drink=Coca-Cola Zero" \{'\n'}
              {'  '}<span className="tk-fl">--option</span> "Side=Fries L" \{'\n'}
              {'  '}<span className="tk-fl">--count</span> 1
            </code>
          </pre>
        </li>
        <li className="step">
          <header>
            <span className="step__n">05</span>
            <h3>Preview the checkout</h3>
          </header>
          <p>Project items, fees, and totals — nothing is submitted.</p>
          <pre className="snippet snippet--dark">
            <code>
              <span className="tk-fn">wolt</span> cart <span className="tk-fl">--details</span> <span className="tk-fl">--venue-id</span> &lt;venue-id&gt;{'\n'}
              <span className="tk-fn">wolt</span> checkout <span className="tk-fl">--delivery-mode</span> standard <span className="tk-fl">--venue-id</span> &lt;venue-id&gt;
            </code>
          </pre>
        </li>
      </ol>
    </section>
  );
}

function Commands() {
  const cards: Array<{name: string; body: ReactNode}> = [
    {name: 'wolt login / logout / status', body: 'Browser-driven login via managed Chrome. Manual token fallback. Automatic refresh with actionable auth errors.'},
    {name: 'wolt feed', body: 'Section-grouped discovery home page. Add --summary for a one-line-per-section overview.'},
    {name: 'wolt top', body: 'Flatten the feed into a single top-N ranked table. Default 10. Dedupes by venue.'},
    {name: 'wolt venues', body: 'Flat list with filters: --query, --sort, --open-now, --wolt-plus, --promotions-only, pagination.'},
    {name: 'wolt venue', body: 'Details, static venue-local hours, categories, menu (full / --query / --category), single-item drilldown.'},
    {
      name: 'wolt cart',
      body: (
        <>
          <code>cart</code> · <code>count</code> · <code>add</code> ·{' '}
          <code>remove</code> · <code>clear</code>. Add by id, URL, or name.
        </>
      ),
    },
    {name: 'wolt checkout', body: 'Project totals, fees, and delivery cost from the current cart. No order placement.'},
    {name: 'wolt account', body: 'Profile · orders · addresses · payments · favourites. Read-only by default.'},
    {
      name: 'wolt stats',
      body: (
        <>
          Download the <code>wolt-stats</code> dashboard bundle, sync history
          into local SQLite (pure Go, no Node), serve on{' '}
          <code>127.0.0.1:5173</code>, and open the browser.
        </>
      ),
    },
  ];

  return (
    <section id="commands" className="commands">
      <header className="section-head">
        <span className="section-head__eyebrow">Command surface</span>
        <h2 className="section-head__title">Every group at a glance.</h2>
      </header>

      <div className="cmd-grid">
        {cards.map((c) => (
          <article key={c.name} className="cmd-card">
            <h4>
              <code>{c.name}</code>
            </h4>
            <p>{c.body}</p>
          </article>
        ))}
      </div>

      <details className="flags">
        <summary>Global flags reference</summary>
        <div className="flags__grid">
          <div><code>--format</code><span>table · json · yaml</span></div>
          <div><code>--address</code><span>temporary location, geocoded</span></div>
          <div><code>--lat / --lon</code><span>coordinate override (paired)</span></div>
          <div><code>--locale</code><span>BCP-47 locale tag</span></div>
          <div><code>--no-color</code><span>disable ANSI colors</span></div>
          <div><code>--verbose</code><span>HTTP trace + diagnostics</span></div>
          <div><code>--limit / --offset / --page</code><span>pagination on list commands</span></div>
          <div><code>--show-highlights</code><span>force / auto / hide Highlights column</span></div>
          <div><code>WOLT_BADGES_PLAIN=1</code><span>plain-text badge labels</span></div>
        </div>
      </details>
    </section>
  );
}

function Agents() {
  const agents: Array<{name: string; tag: string; body: ReactNode; icon: ReactNode; accent: string}> = [
    {
      name: 'Claude Code',
      tag: 'Anthropic · CLI',
      accent: 'cyan',
      body: (
        <>
          Pipe <code>wolt feed --format json</code> straight into prompts. The
          flag surface is small and stable enough for Claude to drive cart
          building end-to-end without supervision.
        </>
      ),
      icon: (
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M12 3v18M3 12h18M5.6 5.6l12.8 12.8M18.4 5.6 5.6 18.4"
          />
        </svg>
      ),
    },
    {
      name: 'Claude Desktop',
      tag: 'native MCP',
      accent: 'blue',
      body: (
        <>
          Drop the one-liner below into{' '}
          <code>claude_desktop_config.json</code> and Claude gets 25 typed
          tools — feed, search, venue menu, cart, checkout preview — backed
          by the bundled <code>wolt-mcp</code> binary. No wrappers, no glue
          code.
        </>
      ),
      icon: (
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <rect x="3" y="5" width="18" height="13" rx="2" fill="none" stroke="currentColor" strokeWidth="1.6" />
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M8 21h8M12 18v3M8 11l2.5 2.5L15 9"
          />
        </svg>
      ),
    },
    {
      name: 'OpenClaw',
      tag: 'self-hosted',
      accent: 'orange',
      body: (
        <>
          Drop a 5-line AgentSkill that shells out to <code>wolt</code>. The
          Markdown personality model means scope, memory, and permissions sit
          alongside your CLI calls — no glue code needed.
        </>
      ),
      icon: (
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M4 9c2-3 5-5 8-5s6 2 8 5M4 9c0 4 3 7 8 7s8-3 8-7M9 16v2a2 2 0 0 0 2 2h2a2 2 0 0 0 2-2v-2M3 11l-1 2M21 11l1 2"
          />
          <circle cx="9.5" cy="9.5" r="0.9" fill="currentColor" />
          <circle cx="14.5" cy="9.5" r="0.9" fill="currentColor" />
        </svg>
      ),
    },
    {
      name: 'PicoClaw',
      tag: '10 MB · MCP',
      accent: 'pink',
      body: (
        <>
          A 10 MB Go-binary AI agent meets a Go-binary CLI. Native MCP support
          means you can wire <code>wolt</code> onto a Raspberry Pi or a $10
          RISC-V board and drive carts from your sofa.
        </>
      ),
      icon: (
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <rect x="6" y="6" width="12" height="12" rx="2" fill="none" stroke="currentColor" strokeWidth="1.6" />
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            d="M9 3v3M15 3v3M9 18v3M15 18v3M3 9h3M3 15h3M18 9h3M18 15h3"
          />
          <circle cx="12" cy="12" r="2" fill="currentColor" />
        </svg>
      ),
    },
    {
      name: 'Cursor',
      tag: 'AI IDE',
      accent: 'mono',
      body: (
        <>
          Ask Cursor's Agent to drive <code>wolt cart add …</code> from the
          chat side-panel. Tool calls land in your terminal exactly as a human
          would type them.
        </>
      ),
      icon: (
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M5 3v18l5-5 3 7 2.6-1-3-7h6L5 3Z"
          />
        </svg>
      ),
    },
    {
      name: 'Cline',
      tag: 'VS Code',
      accent: 'green',
      body: (
        <>
          Shell-first agent inside VS Code. Reads <code>wolt --help</code>{' '}
          once, then drives discovery, cart, and checkout preview from the
          chat panel.
        </>
      ),
      icon: (
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.8"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M5 7l5 5-5 5M13 17h7"
          />
        </svg>
      ),
    },
    {
      name: 'Codex CLI',
      tag: 'OpenAI',
      accent: 'cyan',
      body: (
        <>
          OpenAI's shell-native coder. Pipe <code>wolt --format json</code>{' '}
          straight into a prompt, or attach the <code>wolt-mcp</code> server
          for typed tool calls. Both work; the shell flow is one command.
        </>
      ),
      icon: (
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeWidth="1.6" />
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            d="M8.5 9l7 6M15.5 9l-7 6"
          />
        </svg>
      ),
    },
    {
      name: 'Aider',
      tag: 'pair CLI',
      accent: 'yellow',
      body: (
        <>
          Pure-terminal pair-programmer. Point it at a repo and it'll read the
          docs, scaffold a script, and run <code>wolt</code> to confirm the
          shape of the output it just generated.
        </>
      ),
      icon: (
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <circle cx="12" cy="12" r="8" fill="none" stroke="currentColor" strokeWidth="1.6" />
          <circle cx="9" cy="11" r="1.3" fill="currentColor" />
          <circle cx="15" cy="11" r="1.3" fill="currentColor" />
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            d="M9 16c1 .8 2 1.2 3 1.2s2-.4 3-1.2"
          />
        </svg>
      ),
    },
  ];

  const {copiedKey, copy} = useCopy();
  const mcpConfigText = `{\n  "mcpServers": {\n    "wolt": { "command": "wolt-mcp" }\n  }\n}`;
  const mcpCopied = copiedKey === 'agents-mcp';
  const demoText = [
    '# 1. Discover',
    'wolt venues --query ramen --sort rating --format json',
    '',
    '# 2. Read the menu',
    'wolt venue menu <slug> --query tonkotsu --format json',
    '',
    '# 3. Build the cart by name',
    'wolt cart add <slug> --query "Tonkotsu Ramen"',
    'wolt cart add <slug> --query "Genmaicha"',
    '',
    '# 4. Preview the checkout (no order placed)',
    'wolt checkout --format json',
  ].join('\n');
  const demoCopied = copiedKey === 'agents-demo';

  return (
    <section id="agents" className="agents">
      <header className="section-head">
        <span className="section-head__eyebrow">
          <span className="section-head__pulse" aria-hidden="true" />
          Works with your agent
        </span>
        <h2 className="section-head__title">
          Drive wolt-cli from <span className="grad">your AI of choice.</span>
        </h2>
        <p className="section-head__lede">
          Two ways in. Pipe <code>wolt --format json</code> straight into a
          shell-driving agent (Codex CLI, Cursor, Cline, Aider), or wire up
          the bundled <code>wolt-mcp</code> server for typed{' '}
          <a href="https://modelcontextprotocol.io" target="_blank" rel="noreferrer">
            Model Context Protocol
          </a>{' '}
          tool calls in Claude Desktop, Claude Code, and any other MCP host.
        </p>
      </header>

      <div className="agents__mcp" aria-label="MCP wiring">
        <div className="agents__mcp-copy">
          <span className="agents__mcp-eyebrow">Plug in once</span>
          <h3 className="agents__mcp-title">
            One config line, 25 typed tools.
          </h3>
          <p>
            <code>wolt-mcp</code> ships in the same Homebrew formula as the
            CLI. It shares the same login on disk, so one{' '}
            <code>wolt login</code> unlocks every auth-gated tool —{' '}
            <code>wolt_cart_show</code>, <code>wolt_account_orders</code>,{' '}
            <code>wolt_checkout_preview</code>, and all the rest.
          </p>
          <a className="btn btn--ghost" href="https://github.com/mekedron/wolt-cli/blob/main/docs/mcp.md">
            Full tool catalog →
          </a>
        </div>
        <div className="codeblock">
          <pre>
            <code>
              {'{'}{'\n'}
              {'  '}<span className="tk-st">"mcpServers"</span>: {'{'}{'\n'}
              {'    '}<span className="tk-st">"wolt"</span>: {'{ '}<span className="tk-st">"command"</span>: <span className="tk-st">"wolt-mcp"</span>{' }'}{'\n'}
              {'  '}{'}'}{'\n'}
              {'}'}
            </code>
          </pre>
          <button
            type="button"
            className={`codeblock__copy${mcpCopied ? ' is-copied' : ''}`}
            onClick={() => copy('agents-mcp', mcpConfigText)}>
            {mcpCopied ? 'Copied' : 'Copy'}
          </button>
        </div>
      </div>

      <div className="agents__grid">
        {agents.map((a) => (
          <article key={a.name} className={`agent-card agent-card--${a.accent}`}>
            <div className="agent-card__icon" aria-hidden="true">
              {a.icon}
            </div>
            <div className="agent-card__body">
              <div className="agent-card__head">
                <h4>{a.name}</h4>
                <span className="agent-card__tag">{a.tag}</span>
              </div>
              <p>{a.body}</p>
            </div>
          </article>
        ))}
      </div>

      <div className="agents__why" aria-label="Why wolt-cli is agent-friendly">
        <div className="agents__why-cell">
          <span className="agents__why-k">Native MCP server</span>
          <span className="agents__why-v">25 typed tools, stdio transport, official MCP Go SDK</span>
        </div>
        <div className="agents__why-cell">
          <span className="agents__why-k">Single binary</span>
          <span className="agents__why-v">zero deps, any host the agent can shell into</span>
        </div>
        <div className="agents__why-cell">
          <span className="agents__why-k">json / yaml on every command</span>
          <span className="agents__why-v">deterministic parsing across lists, carts, and previews</span>
        </div>
        <div className="agents__why-cell">
          <span className="agents__why-k">Stable --help surface</span>
          <span className="agents__why-v">LLMs can read it once and remember the shape</span>
        </div>
        <div className="agents__why-cell">
          <span className="agents__why-k">No telemetry</span>
          <span className="agents__why-v">tokens stay local, nothing phones home</span>
        </div>
      </div>

      <div className="agents__demo">
        <header className="agents__demo-head">
          <span className="agents__demo-eyebrow">Sample agent run</span>
          <span className="agents__demo-tag">
            "find me top-rated ramen and build a cart"
          </span>
        </header>
        <div className="codeblock">
          <pre>
            <code>
              <span className="tk-mut"># 1. Discover</span>{'\n'}
              <span className="tk-fn">wolt</span> venues <span className="tk-fl">--query</span> ramen <span className="tk-fl">--sort</span> rating <span className="tk-fl">--format</span> json{'\n'}
              {'\n'}
              <span className="tk-mut"># 2. Read the menu</span>{'\n'}
              <span className="tk-fn">wolt</span> venue menu &lt;slug&gt; <span className="tk-fl">--query</span> tonkotsu <span className="tk-fl">--format</span> json{'\n'}
              {'\n'}
              <span className="tk-mut"># 3. Build the cart by name</span>{'\n'}
              <span className="tk-fn">wolt</span> cart add &lt;slug&gt; <span className="tk-fl">--query</span> <span className="tk-st">"Tonkotsu Ramen"</span>{'\n'}
              <span className="tk-fn">wolt</span> cart add &lt;slug&gt; <span className="tk-fl">--query</span> <span className="tk-st">"Genmaicha"</span>{'\n'}
              {'\n'}
              <span className="tk-mut"># 4. Preview the checkout (no order placed)</span>{'\n'}
              <span className="tk-fn">wolt</span> checkout <span className="tk-fl">--format</span> json
            </code>
          </pre>
          <button
            type="button"
            className={`codeblock__copy${demoCopied ? ' is-copied' : ''}`}
            onClick={() => copy('agents-demo', demoText)}>
            {demoCopied ? 'Copied' : 'Copy'}
          </button>
        </div>
        <p className="agents__demo-note">
          The CLI is read-only by default and <strong>never</strong> places a
          real order — agents can browse, build carts, and project totals
          without spending your money. <code>wolt checkout</code> is a
          preview.
        </p>
      </div>
    </section>
  );
}

function FAQ() {
  const qas: Array<{q: string; a: ReactNode; open?: boolean}> = [
    {
      q: 'Is this an official Wolt product?',
      open: true,
      a: (
        <>
          No. wolt-cli is an <strong>unofficial, community-built</strong> Go CLI. It's
          not affiliated with, endorsed by, or supported by Wolt. Use it at your own
          responsibility, and respect their terms of service.
        </>
      ),
    },
    {
      q: 'Can it place real orders?',
      a: (
        <>
          No. The CLI exposes <code>wolt checkout</code> as a preview only, which
          projects totals and fees. Final order placement still happens in the
          official Wolt app or website, using the delivery address selected in
          your account.
        </>
      ),
    },
    {
      q: 'Does it ship an MCP server?',
      a: (
        <>
          Yes. The companion <code>wolt-mcp</code> binary is installed
          alongside <code>wolt</code> by the Homebrew formula and exposes 25
          typed Model Context Protocol tools (discovery, venue, account,
          favorites, cart, checkout preview). Wire it in with{' '}
          <code>{`{ "mcpServers": { "wolt": { "command": "wolt-mcp" } } }`}</code>
          {' '}— see{' '}
          <a href="https://github.com/mekedron/wolt-cli/blob/main/docs/mcp.md">
            docs/mcp.md
          </a>{' '}
          for the full catalog and per-client wiring.
        </>
      ),
    },
    {
      q: 'Where is my config stored?',
      a: (
        <>
          By default at <code>~/.wolt/.wolt-config.json</code>, or wherever{' '}
          <code>WOLT_CONFIG_PATH</code> points. The file may contain <code>wtoken</code>
          , <code>wrtoken</code>, and cookies — keep it local and don't commit it. The
          project's <code>.gitignore</code> already ignores common config patterns.
        </>
      ),
    },
    {
      q: 'What about location overrides?',
      a: (
        <>
          Pass <code>--address</code> or <code>--lat/--lon</code> per command. They
          affect preview inputs only. Final orders still use your Wolt-saved address.{' '}
          <code>--lat</code> and <code>--lon</code> must be supplied together;{' '}
          <code>--address</code> can't be combined with them.
        </>
      ),
    },
    {
      q: 'Which platforms are supported?',
      a: 'Anywhere Go 1.26+ builds: macOS, Linux, and Windows. Homebrew is the smoothest path on macOS and Linux.',
    },
    {
      q: 'How do I report a bug or contribute?',
      a: (
        <>
          Open an issue or PR on the GitHub repository. Run <code>go test ./...</code>{' '}
          and <code>make lint</code> before submitting.
        </>
      ),
    },
  ];

  return (
    <section id="faq" className="faq">
      <header className="section-head">
        <span className="section-head__eyebrow">Honest answers</span>
        <h2 className="section-head__title">FAQ</h2>
      </header>

      <div className="faq__list">
        {qas.map((qa) => (
          <details key={qa.q} className="qa" open={qa.open}>
            <summary>{qa.q}</summary>
            <p>{qa.a}</p>
          </details>
        ))}
      </div>
    </section>
  );
}

function CTA() {
  return (
    <section className="cta">
      <div className="cta__inner">
        <h2>Try it in 30 seconds.</h2>
        <p>
          One <code>brew install</code>, one <code>wolt login</code>, then{' '}
          <code>wolt top 10</code>. That's the whole flow.
        </p>
        <InlineCmd id="cta-cmd" size="lg" text="brew install mekedron/tap/wolt-cli">
          <span className="tk-fn">brew</span> install mekedron/tap/wolt-cli
        </InlineCmd>
        <a
          className="btn btn--primary btn--lg"
          href="https://github.com/mekedron/wolt-cli"
          target="_blank"
          rel="noopener noreferrer">
          View on GitHub →
        </a>
      </div>
    </section>
  );
}

function Support() {
  return (
    <section id="support" className="support" aria-labelledby="support-h">
      <div className="support__inner">
        <div className="support__copy">
          <span className="support__badge">
            <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true">
              <path
                fill="currentColor"
                d="M12 21s-7-4.5-7-10a4.5 4.5 0 0 1 8-2.8A4.5 4.5 0 0 1 19 11c0 5.5-7 10-7 10Z"
              />
            </svg>
            Community-built
          </span>
          <h2 id="support-h" className="support__title">
            If wolt-cli saved you a tab, buy me a coffee.
          </h2>
          <p className="support__lede">
            wolt-cli is built and maintained by one developer in their free time — MIT
            licensed, free forever, no telemetry, no upsells. If it makes your terminal
            a little better, a small tip keeps it caffeinated.
          </p>
          <div className="support__ctas">
            <a
              className="btn btn--coffee btn--lg"
              href="https://buymeacoffee.com/mekedron"
              target="_blank"
              rel="noopener noreferrer">
              <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
                <path
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.8"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M5 10h12v5a4 4 0 0 1-4 4H9a4 4 0 0 1-4-4v-5Zm12 1h2a2.5 2.5 0 0 1 0 5h-2M8 3c0 1 1 1 1 2s-1 1-1 2M12 3c0 1 1 1 1 2s-1 1-1 2"
                />
              </svg>
              Buy me a coffee
            </a>
            <a
              className="btn btn--ghost btn--lg btn--on-warm"
              href="https://github.com/mekedron/wolt-cli"
              target="_blank"
              rel="noopener noreferrer">
              <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
                <path
                  fill="currentColor"
                  d="M12 .5A11.5 11.5 0 0 0 .5 12a11.5 11.5 0 0 0 7.86 10.92c.57.1.78-.25.78-.55v-2.1c-3.2.7-3.87-1.36-3.87-1.36-.53-1.34-1.3-1.7-1.3-1.7-1.06-.72.08-.71.08-.71 1.17.08 1.79 1.2 1.79 1.2 1.04 1.78 2.73 1.27 3.4.97.1-.75.4-1.27.74-1.56-2.55-.29-5.24-1.28-5.24-5.7 0-1.26.45-2.29 1.18-3.1-.12-.29-.51-1.46.11-3.04 0 0 .97-.31 3.18 1.19a11 11 0 0 1 5.8 0c2.2-1.5 3.17-1.19 3.17-1.19.63 1.58.23 2.75.12 3.04.73.81 1.18 1.84 1.18 3.1 0 4.43-2.7 5.4-5.27 5.69.41.36.78 1.05.78 2.12v3.14c0 .3.21.66.79.55A11.5 11.5 0 0 0 23.5 12 11.5 11.5 0 0 0 12 .5Z"
                />
              </svg>
              Star on GitHub
            </a>
          </div>
          <p className="support__credit">
            By{' '}
            <a href="https://github.com/mekedron" target="_blank" rel="noopener noreferrer">
              @mekedron
            </a>{' '}
            · 100% of tips go to keeping the project alive.
          </p>
        </div>

        <aside className="support__card" aria-label="What support unlocks">
          <header>
            <span className="support__card-eyebrow">What your tip funds</span>
          </header>
          <ul className="support__list">
            <li>
              <span className="support__dot" aria-hidden="true" />
              <div>
                <strong>New commands</strong>
                <span>discovery filters, batch carts, exports</span>
              </div>
            </li>
            <li>
              <span className="support__dot" aria-hidden="true" />
              <div>
                <strong>API drift fixes</strong>
                <span>quick patches when endpoints change</span>
              </div>
            </li>
            <li>
              <span className="support__dot" aria-hidden="true" />
              <div>
                <strong>Docs &amp; examples</strong>
                <span>real recipes, jq one-liners, scripts</span>
              </div>
            </li>
            <li>
              <span className="support__dot" aria-hidden="true" />
              <div>
                <strong>Coffee, honestly</strong>
                <span>literal coffee. it helps. a lot.</span>
              </div>
            </li>
          </ul>
        </aside>
      </div>
    </section>
  );
}

export default function Home(): ReactNode {
  // useBaseUrl is invoked at render time so the build resolves /wolt-cli/ prefix correctly.
  useBaseUrl('/');
  return (
    <Layout
      title="wolt-cli — Unofficial Wolt CLI for the terminal"
      description="wolt-cli is an unofficial community Go CLI for browsing venues, menus, and carts from your terminal. Install with Homebrew, configure once, and shop without leaving the shell.">
      <div className="wcli-home">
        <Hero />
        <Trust />
        <Features />
        <Stats />
        <Install />
        <Example />
        <Commands />
        <Agents />
        <FAQ />
        <CTA />
        <Support />
      </div>
    </Layout>
  );
}
