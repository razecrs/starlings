# Arena results

Medians, median absolute deviation, and deterministic 95% bootstrap confidence intervals. Relative speed is calculated only inside an identical workload and coverage class.

The recorded run used WSL2 on an i7-9750H. This repository retains the aggregate report, source locks, fixtures, and adapters, but not the original JSONL rows; rerun the arena before treating these numbers as independent verification.

| Workload | Coverage | Library | Samples | Median | 95% CI | MAD | Peak RSS | Relative |
|---|---|---|---:|---:|---:|---:|---:|---:|
| cold_start | library_load | nim_dimscord | 30 | 29.301 ms | 28.692 ms–29.872 ms | 772.900 µs | 7.3 MiB | 1.00x |
| cold_start | library_load | crystal_discordcr | 30 | 58.775 ms | 54.142 ms–61.059 ms | 5.559 ms | 10.3 MiB | 2.01x |
| cold_start | library_load | php_restcord | 30 | 65.761 ms | 61.957 ms–66.890 ms | 3.804 ms | 33.2 MiB | 2.24x |
| cold_start | library_load | dart_nyxx | 30 | 70.421 ms | 65.130 ms–73.916 ms | 6.150 ms | 7.4 MiB | 2.40x |
| cold_start | library_load | php_discord-php | 30 | 78.621 ms | 77.430 ms–80.912 ms | 2.234 ms | 35.2 MiB | 2.68x |
| cold_start | library_load | go_discordgo | 30 | 84.588 ms | 82.053 ms–87.054 ms | 5.300 ms | 7.8 MiB | 2.89x |
| cold_start | library_load | java_discord4j | 30 | 99.972 ms | 98.635 ms–102.117 ms | 2.320 ms | 41.4 MiB | 3.41x |
| cold_start | library_load | java_jda | 30 | 104.993 ms | 102.319 ms–107.025 ms | 4.000 ms | 41.4 MiB | 3.58x |
| cold_start | library_load | lua_discordia | 30 | 109.534 ms | 107.177 ms–111.533 ms | 2.974 ms | 10.5 MiB | 3.74x |
| cold_start | library_load | starlings | 30 | 126.399 ms | 122.410 ms–132.075 ms | 7.806 ms | 8.3 MiB | 4.31x |
| cold_start | library_load | kotlin_kord | 30 | 149.844 ms | 146.880 ms–154.021 ms | 6.345 ms | 43.2 MiB | 5.11x |
| cold_start | library_load | scala_ackcord | 30 | 166.806 ms | 154.196 ms–179.791 ms | 17.257 ms | 41.7 MiB | 5.69x |
| cold_start | library_load | dotnet_dsharpplus | 30 | 452.573 ms | 425.312 ms–466.641 ms | 27.261 ms | 57.0 MiB | 15.45x |
| cold_start | library_load | js_discord-js | 30 | 555.233 ms | 524.117 ms–615.929 ms | 56.171 ms | 91.9 MiB | 18.95x |
| cold_start | library_load | python_discord-py | 30 | 630.966 ms | 624.326 ms–643.899 ms | 13.138 ms | 50.2 MiB | 21.53x |
| cold_start | library_load | ruby_discordrb | 30 | 786.929 ms | 776.499 ms–823.169 ms | 26.939 ms | 51.4 MiB | 26.86x |
| cold_start | library_load | elixir_nostrum | 30 | 945.336 ms | 919.067 ms–975.877 ms | 38.240 ms | 93.1 MiB | 32.26x |
| guild_create_state | decode_only | nim_dimscord | 30 | 5.322 ms | 5.272 ms–5.352 ms | 95.851 µs | 7.6 MiB | 1.00x |
| guild_create_state | state_update | js_discord-js | 30 | 1.003 ms | 970.691 µs–1.051 ms | 52.435 µs | 121.9 MiB | 1.00x |
| guild_create_state | state_update | starlings | 30 | 2.098 ms | 2.065 ms–2.142 ms | 53.227 µs | 20.2 MiB | 2.09x |
| guild_create_state | state_update | go_discordgo | 30 | 2.592 ms | 2.505 ms–2.785 ms | 157.785 µs | 13.5 MiB | 2.58x |
| guild_create_state | state_update | python_discord-py | 30 | 3.159 ms | 3.122 ms–3.257 ms | 75.652 µs | 51.5 MiB | 3.15x |
| guild_create_state | state_update | dotnet_dsharpplus | 30 | 9.994 ms | 9.789 ms–10.353 ms | 518.081 µs | 109.1 MiB | 9.97x |
| member_lookup | public_cache | dotnet_dsharpplus | 30 | 3.5 ns | 3.4 ns–3.5 ns | 0.1 ns | 84.6 MiB | 1.00x |
| member_lookup | public_cache | js_discord-js | 30 | 98.5 ns | 96.1 ns–105.4 ns | 4.7 ns | 111.9 MiB | 28.25x |
| member_lookup | public_cache | go_discordgo | 30 | 117.8 ns | 117.1 ns–119.0 ns | 1.5 ns | 13.5 MiB | 33.79x |
| member_lookup | public_cache | starlings | 30 | 173.2 ns | 168.8 ns–181.5 ns | 6.5 ns | 14.4 MiB | 49.68x |
| member_lookup | public_cache | python_discord-py | 30 | 316.4 ns | 304.4 ns–332.2 ns | 15.8 ns | 51.3 MiB | 90.76x |
| message_handled | full_dispatch | starlings | 30 | 7.615 µs | 7.379 µs–7.766 µs | 344.7 ns | 14.4 MiB | 1.00x |
| message_handled | full_dispatch | js_discord-js | 30 | 8.387 µs | 8.330 µs–8.913 µs | 357.1 ns | 116.4 MiB | 1.10x |
| message_handled | full_dispatch | python_discord-py | 30 | 19.633 µs | 19.098 µs–20.142 µs | 692.4 ns | 51.5 MiB | 2.58x |
| message_handled | full_dispatch | dotnet_dsharpplus | 30 | 26.012 µs | 23.987 µs–28.283 µs | 2.846 µs | 105.2 MiB | 3.42x |
| message_handled | state_update | go_discordgo | 30 | 11.953 µs | 11.846 µs–12.295 µs | 313.9 ns | 13.3 MiB | 1.00x |
| message_unhandled | full_dispatch | js_discord-js | 30 | 7.967 µs | 7.819 µs–8.292 µs | 294.2 ns | 115.9 MiB | 1.00x |
| message_unhandled | full_dispatch | python_discord-py | 30 | 19.828 µs | 18.988 µs–21.918 µs | 1.436 µs | 51.3 MiB | 2.49x |
| message_unhandled | full_dispatch | dotnet_dsharpplus | 30 | 22.350 µs | 21.820 µs–25.228 µs | 1.081 µs | 104.3 MiB | 2.81x |
| message_unhandled | state_update | starlings | 30 | 7.776 µs | 7.549 µs–7.914 µs | 303.8 ns | 14.1 MiB | 1.00x |
| message_unhandled | state_update | go_discordgo | 30 | 12.357 µs | 12.104 µs–12.702 µs | 465.8 ns | 13.5 MiB | 1.59x |
| permission_resolve | public_cache | starlings | 30 | 191.8 ns | 190.5 ns–196.7 ns | 4.1 ns | 10.1 MiB | 1.00x |
| permission_resolve | public_cache | go_discordgo | 30 | 410.1 ns | 404.6 ns–416.0 ns | 8.7 ns | 13.2 MiB | 2.14x |
| permission_resolve | public_cache | dotnet_dsharpplus | 30 | 1.445 µs | 717.0 ns–1.736 µs | 735.5 ns | 92.5 MiB | 7.53x |
| permission_resolve | public_cache | js_discord-js | 30 | 1.726 µs | 1.694 µs–1.764 µs | 43.6 ns | 112.2 MiB | 9.00x |
| permission_resolve | public_cache | python_discord-py | 30 | 3.989 µs | 3.945 µs–4.176 µs | 158.3 ns | 51.3 MiB | 20.80x |

## Public benchmark coverage

A dash means the pinned library does not expose a supported public offline path for that workload; it was not assigned a fake score.

| Library | Handled dispatch | Unhandled dispatch | Guild state | Member lookup | Permissions | Total |
|---|:---:|:---:|:---:|:---:|:---:|---:|
| dotnet_dsharpplus | yes | yes | yes | yes | yes | 5/5 |
| go_discordgo | yes | yes | yes | yes | yes | 5/5 |
| js_discord-js | yes | yes | yes | yes | yes | 5/5 |
| python_discord-py | yes | yes | yes | yes | yes | 5/5 |
| starlings | yes | yes | yes | yes | yes | 5/5 |
| nim_dimscord | — | — | yes | — | — | 1/5 |
| crystal_discordcr | — | — | — | — | — | 0/5 |
| dart_nyxx | — | — | — | — | — | 0/5 |
| elixir_nostrum | — | — | — | — | — | 0/5 |
| java_discord4j | — | — | — | — | — | 0/5 |
| java_jda | — | — | — | — | — | 0/5 |
| kotlin_kord | — | — | — | — | — | 0/5 |
| lua_discordia | — | — | — | — | — | 0/5 |
| php_discord-php | — | — | — | — | — | 0/5 |
| php_restcord | — | — | — | — | — | 0/5 |
| ruby_discordrb | — | — | — | — | — | 0/5 |
| scala_ackcord | — | — | — | — | — | 0/5 |
