<h1>🍔 What to eat? 🍕</h1>
<p align="center">
    <em>CLI tool to query Wolt API in your location!</em>
</p>

![Release](https://github.com/Valaraucoo/wolt-cli/actions/workflows/release.yml/badge.svg)
![Build Status](https://github.com/Valaraucoo/wolt-cli/actions/workflows/tests.yml/badge.svg)
[![PyPI](https://img.shields.io/pypi/v/wolt-cli.svg)](https://pypi.python.org/pypi/wolt-cli/)
[![Code style: black](https://img.shields.io/badge/code%20style-black-000000.svg)](https://github.com/ambv/black)



Why to use *wolt-cli*? How many times have you not known what to order for dinner or lunch? *wolt-cli* will help you querying and filtering restaurants available in your location via [Wolt](https://wolt.com/pl/discovery) app! 🍔

Example usage:

<p align="center">
    <img src="https://raw.githubusercontent.com/Valaraucoo/wolt-cli/master/images/ls-query-example.png" alt="demo" width="900"/>
</p>

<h2>✨ Features </h2>

* 🍔 Query restaurants in your location
* 🍕 Filter restaurants by name, cuisine, price, rating, delivery time, etc.
* 🍗 Display restaurant details
* 🍟 Random restaurant draw

<h2>🛠️ Installation</h2>

*What to eat* is compatible with Python 3.12+ and runs on Linux, macOS and Windows. The latest releases with binary wheels are available from pip. Before you install *What to eat* and its dependencies, make sure that your pip, setuptools and wheel are up to date.

You can install `wolt-cli` using [pip](https://pypi.org/project/wolt-cli/):

```console
pip install wolt-cli
```

<h2>📌 Wolt CLI v1 Documentation (Design)</h2>

This repository now includes a full command reference for a new `wolt` CLI surface.
Implementation comes later; documentation is the source of truth for the interface.

Primary binary:
- `wolt`

Compatibility alias:
- `wolt-cli`

Global flags expected on every v1 command:
- `--format [table|json|yaml]` (default `table`)
- `--profile <name>`
- `--locale <bcp47>`
- `--no-color`
- `--output <path>`

Quick examples (design target):

```console
$ wolt search venues --query burger --limit 10 --format json
$ wolt venue menu burger-king-finnoo --include-options --format yaml
$ wolt cart add 629f1f18480882d6f02c25f0 676939cb70769df4cec6cc6f --count 1 --format json
$ wolt checkout quote --delivery-mode standard --address-id 6916f0f4cbcd388b5e76b8d7 --format yaml
$ wolt orders list --limit 20 --format json
```

v1 command groups:
- `auth`
- `discover`
- `search`
- `venue`
- `item`
- `cart`
- `checkout` (preview/quote only, no order placement in v1)
- `orders`
- `profile`

Detailed design docs:

| Documentation | Scope |
|---|---|
| [CLI overview](./docs/cli-overview.md) | Information architecture, safety model, migration context |
| [Output contract](./docs/cli-output-contract.md) | `json`/`yaml` envelope, canonical schemas, error format |
| [Auth commands](./docs/cli-auth.md) | `auth status`, `auth login`, `auth logout` |
| [Discovery and search commands](./docs/cli-discovery-search.md) | `discover *`, `search *` |
| [Venue and item commands](./docs/cli-venue-item.md) | `venue *`, `item show` |
| [Cart and checkout commands](./docs/cli-cart-checkout.md) | `cart *`, `checkout preview|delivery-modes|quote` |
| [Orders and profile commands](./docs/cli-orders-profile.md) | `orders *`, `profile *` |

<h2>💬 Current implementation commands (legacy surface)</h2>

Currently shipped commands in `wolt-cli` are: `configure`, `ls`, `discover`, `search`, `venue`, and `item`.

```console
$ wolt-cli --help

 Usage: wolt-cli [OPTIONS] COMMAND [ARGS]...

 Browse Wolt venues, inspect menus, and manage local profiles.

╭─ Options ───────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│ --version             -v        Show CLI version and exit.                                                                  │
│ --install-completion            Install completion for the current shell.                                                   │
│ --show-completion               Show completion for the current shell, to copy it or customize the installation.            │
│ --help                          Show this message and exit.                                                                 │
╰─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
╭─ Commands ──────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│ configure                                Create and manage local profile configuration.                                     │
│ discover                                 Read discovery feed and browse categories.                                         │
│ item                                     Inspect a single menu item for a venue.                                            │
│ ls                                       List restaurants near the selected profile location.                               │
│ search                                   Search venues and menu items by query.                                             │
│ venue                                    Inspect venue details, menus, and opening hours.                                  │
╰─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯

```

You can find examples of using these commands in the section below.


<h2>✨ Examples</h2>
Configure your tool:

```console
$ wolt-cli configure
```


List all available restaurants in your localization:

```console
$ wolt-cli ls
```


Sort restaurants by `rating` and limit results to 5 records:
```console
$ wolt-cli ls --sort rating --ordering desc --limit 5
┏━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━┳━━━━━━━━┳━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━┓
┃ No. ┃                               Restaurant ┃                  Address ┃ Estimate time ┃ Delivery cost ┃ Rating ┃ Price ┃                Tags ┃
┡━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━╇━━━━━━━━╇━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━┩
│ 1   │               Mikropiekarnia Pochlebstwo │       Romanowicza 5/LU7b │   25 - 35 min │ (No delivery) │   10.0 │  💰💰 │     Bakery, Grocery │
│ 2   │                            KruKam Kraków │        ul. Krakowska 35A │   30 - 40 min │ (No delivery) │    9.8 │  💰💰 │    Grocery, Healthy │
│ 3   │                    Piekarnia Mojego Taty │           ul. Meiselsa 6 │   20 - 30 min │ (No delivery) │    9.8 │    💰 │     Bakery, Grocery │
│ 4   │  MARLIN - Fish & Chips - Smażalnie Rybne │ Krowoderskich Zuchów 21A │   45 - 55 min │ (No delivery) │    9.6 │  💰💰 │ Fish, Mediterranean │
│ 5   │ Lody Ice Cream NOW - Stare Miasto II (K) │  This is a virtual venue │   20 - 30 min │ (No delivery) │    9.6 │  💰💰 │           Ice cream │
└─────┴──────────────────────────────────────────┴──────────────────────────┴───────────────┴───────────────┴────────┴───────┴─────────────────────┘
                                                        🍿 Restaurants in Kraków via wolt 🍿
```

While using `ls` command you can also use option `query` to filter results by restaurant name, address or tags:

```console
$ wolt-cli ls --query Pizza --limit 3
┏━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━┳━━━━━━━━┳━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━┓
┃ No. ┃                          Restaurant ┃           Address ┃ Estimate time ┃ Delivery cost ┃ Rating ┃ Price ┃                  Tags ┃
┡━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━╇━━━━━━━━╇━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━┩
│ 1   │ Pizzeria Caprese Chillzone Młynówka │    Racławicka 21, │   20 - 30 min │ (No delivery) │    8.4 │  💰💰 │        Italian, pizza │
│ 2   │                            U Filipa │ Ul. Św. Filipa 25 │   30 - 40 min │ (No delivery) │    7.8 │    💰 │                 pizza │
│ 3   │                  Baqaro - Rakowicka │      Rakowicka 11 │   25 - 35 min │ (No delivery) │      - │  💰💰 │ Italian, Pinsa, pizza │
└─────┴─────────────────────────────────────┴───────────────────┴───────────────┴───────────────┴────────┴───────┴───────────────────────┘
                                                   🍿 Restaurants in Kraków via wolt 🍿
```

By default your first profile is `default` one. But while listing restaurants you can change it using `profile` option:

```console
$ wolt-cli ls --profile work
```

You can also display restaurant details by using `ls` command with restaurant name:

```console
$ wolt-cli ls zapiecek
┏━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃     🍕 Zapiecek ┃                       Kraków, Ul. Floriańska 20 🍕 ┃
┡━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┩
│          Rating │                           Amazing (9 / 20 reviews) │
│           Price │                                                 💰 │
│    Opening time │                                      10:00 - 20:45 │
│         Website │ https://wolt.com/pl/pol/krakow/restaurant/zapiecek │
│           Phone │                                      +48 124221345 │
│       Estimates │                                         30 minutes │
│ Payment Methods │                                               Card │
│     Description │               Kultowy bar kanapkowo - sałatkowy... │
│            Tags │                                    Sandwich, Salad │
└─────────────────┴────────────────────────────────────────────────────┘
```

<h2>📖 Documentation </h2>

### v1 design documentation

| Documentation | Command surface |
|---|---|
| [CLI overview](./docs/cli-overview.md) | `wolt <group> <command> [flags]` |
| [Output contract](./docs/cli-output-contract.md) | all commands with `--format json|yaml` |
| [Auth commands](./docs/cli-auth.md) | `wolt auth *` |
| [Discovery and search commands](./docs/cli-discovery-search.md) | `wolt discover *`, `wolt search *` |
| [Venue and item commands](./docs/cli-venue-item.md) | `wolt venue *`, `wolt item show` |
| [Cart and checkout commands](./docs/cli-cart-checkout.md) | `wolt cart *`, `wolt checkout *` |
| [Orders and profile commands](./docs/cli-orders-profile.md) | `wolt orders *`, `wolt profile *` |

### legacy implementation docs (currently shipped)

| Documentation                                             | Command               | Options                                                |
|-----------------------------------------------------------|-----------------------|--------------------------------------------------------|
| [🚀 List all restaurants](./docs/list-all-restaurants.md) | `wolt-cli ls`      | `query`, `profile`, `tag`, `sort`, `ordering`, `limit` |
| 👤 Configure profile                                      | `wolt-cli configure`  |                                                        |

<h2>📚 License</h2>

This project is licensed under the terms of the MIT license.
