<h2>📖 Documentation: List all restaurants</h2>

Command manual:

```console
$ wolt-cli ls --help

 Usage: wolt-cli ls [OPTIONS] [RESTAURANT]

 List restaurants queried from Wolt API.

╭─ Arguments ──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│   restaurant      [RESTAURANT]  Restaurant name [default: None]                                                                                                      │
╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
╭─ Options ────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│ --query     -q      TEXT                                                                    Query search for restaurants [default: None]                             │
│ --profile   -p      TEXT                                                                    Profile name [default: None]                                             │
│ --tag       -t      TEXT                                                                    Tag [default: None]                                                      │
│ --sort      -s      [none|restaurant|-restaurant|address|-address|delivery_cost|-delivery_  Sort by: none, restaurant, -restaurant, address, -address,               │
│                     cost|estimate_time|-estimate_time|rating|-rating|price|-price]          delivery_cost, -delivery_cost, estimate_time, -estimate_time, rating,    │
│                                                                                             -rating, price, -price                                                   │
│                                                                                             [default: Sort.NONE]                                                     │
│ --ordering  -o      [asc|desc]                                                              Ordering: asc, desc [default: Ordering.ASC]                              │
│ --limit     -l      INTEGER                                                                 Limit results [default: None]                                            │
│ --help                                                                                      Show this message and exit.                                              │
╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

By default your first profile is `default` one. But while listing restaurants you can change it using `profile` option:

```console
$ wolt-cli ls --profile work
```

You can query restaurants by name using `query` option and limit the number of results using `limit` option:

```console
$ wolt-cli ls --query pizza --limit 3
┏━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━┳━━━━━━━━┳━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃ No. ┃                   Restaurant ┃             Address ┃ Estimate time ┃ Delivery cost ┃ Rating ┃ Price ┃                     Tags ┃
┡━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━╇━━━━━━━━╇━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━━━┩
│ 1   │                  Pizza Royal │    Chełmońskiego 11 │   50 - 60 min │      9.99 PLN │    8.2 │  💰💰 │           Pizza, Italian │
│ 2   │                      N'Pizza │ Ul. Rajska 3/lok. 2 │   30 - 40 min │      2.49 PLN │    9.2 │  💰💰 │ Italian, Pizza, European │
│ 3   │ Nonna Maria Pizza Napoletana │            Dajwór 9 │   25 - 35 min │      2.49 PLN │    9.2 │  💰💰 │           Italian, Pizza │
└─────┴──────────────────────────────┴─────────────────────┴───────────────┴───────────────┴────────┴───────┴──────────────────────────┘
                                                  🍿 Restaurants in Kraków via wolt 🍿

```

Another cool feature is to list restaurants by tags. You can use `--tag` (or `-t`) option to filter restaurants by tags:

```console
$ wolt-cli ls --tag kebab --limit 3
┏━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━┳━━━━━━━━┳━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃ No. ┃                           Restaurant ┃       Address ┃ Estimate time ┃ Delivery cost ┃ Rating ┃ Price ┃                         Tags ┃
┡━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━╇━━━━━━━━╇━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┩
│ 1   │ Berlin Döner Kebap Galeria Krakowska │       Pawia 5 │   25 - 35 min │      2.49 PLN │    8.2 │    💰 │ Doner, Kebab, Middle eastern │
│ 2   │                                Vegab │ Starowiślna 8 │   25 - 35 min │      2.49 PLN │    9.2 │  💰💰 │           Bowl, Kebab, Vegan │
│ 3   │                Kebaber Starowiślna 8 │ Starowiślna 8 │   20 - 30 min │      2.49 PLN │    8.8 │    💰 │               Kebab, Turkish │
└─────┴──────────────────────────────────────┴───────────────┴───────────────┴───────────────┴────────┴───────┴──────────────────────────────┘
                                                     🍿 Restaurants in Kraków via wolt 🍿

```

By using sorting options you can sort restaurants by name, rating, price, delivery cost and delivery time and chose order (ascending or descending):

```console
$ wolt-cli ls --sort rating --ordering desc --limit 3
┏━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━┳━━━━━━━━┳━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━┓
┃ No. ┃                              Restaurant ┃                  Address ┃ Estimate time ┃ Delivery cost ┃ Rating ┃ Price ┃                  Tags ┃
┡━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━╇━━━━━━━━╇━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━┩
│ 1   │                    Cukiernia Czarodziej │            Karmelicka 15 │   20 - 30 min │ (No delivery) │   10.0 │  💰💰 │             Ice cream │
│ 2   │ MARLIN - Fish & Chips - Smażalnie Rybne │ Krowoderskich Zuchów 21A │   45 - 55 min │ (No delivery) │    9.8 │  💰💰 │   Fish, Mediterranean │
│ 3   │                      Baqaro - Rakowicka │             Rakowicka 11 │   25 - 35 min │      2.49 PLN │    9.8 │  💰💰 │ Italian, Pinsa, Pizza │
└─────┴─────────────────────────────────────────┴──────────────────────────┴───────────────┴───────────────┴────────┴───────┴───────────────────────┘
                                                        🍿 Restaurants in Kraków via wolt 🍿

```

You can also display restaurant details by using `ls` command with restaurant name:

```console
$ wolt-cli ls poco
┏━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃ 🍕 Poco Loco Czysta ┃                                Kraków, Ul. Czysta 9 🍕 ┃
┡━━━━━━━━━━━━━━━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┩
│              Rating │                            Excellent (9 / 200 reviews) │
│               Price │                                                   💰💰 │
│        Opening time │                                          12:00 - 23:00 │
│             Website │    https://wolt.com/pl/pol/krakow/restaurant/poco-loco │
│               Phone │                                          +48 690800805 │
│           Estimates │                                             35 minutes │
│     Payment Methods │                                                   Card │
│         Description │ Zdrowa i lekka kuchnia meksykańska w nowej odsłonie... │
│                Tags │                          Mexican, Taco, Latin american │
└─────────────────────┴────────────────────────────────────────────────────────┘
```

<h2>⚙️ Options </h2>


| Option             | Description                                                                                                                      | Example usage                              |
|--------------------|----------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------|
| `--query`, `-q`    | Command used to query restaurants by `Restaurant Name`, `Address` and `Tags`. Query search is case insensitive.                  | `wolt-cli ls -q pizza`                  |
| `--profile`, `-p`  | Command used to set profile while listing restaurants. By default the `default` profile is used.                                 | `wolt-cli ls -p work`                   |
| `--tag`, `-t`      | Command used to search restaurants by `Tags`. Searching is case insensitive.                                                     | `wolt-cli ls -t italian`                |
| `--sort`, `-s`     | Command used to sort restaurants by one of following fields: `restaurant, address, delivery_cost, estimate_time, rating, price`. | `wolt-cli ls -s rating`                 |
| `--ordering`, `-o` | Command used to order restaurants ascending or descending by field specified in `--sort` option.                                 | `wolt-cli ls -s rating --ordering desc` |
| `--limit`, `-l`    | Command used to limit result to specified number of restaurants.                                                                 | `wolt-cli ls -l 5`                      |
