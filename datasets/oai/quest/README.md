# QUEST OAI data attribution

Bundled file: `gNB_results.csv`

- Project: TUM-LKN QUEST, “User-Based Quality of Service Aware Uplink Resource Scheduling”
- Repository: <https://github.com/tum-lkn/QUEST>
- Source revision: `58e64a649b6e47dbeff10db01ce16c7df0d5e75f`
- Original path: `datasets/users_2/run_1/scheduler_QUEST/gNB_results.csv`
- Original SHA-256: `62d57a8682b45fb918bec804175fbe438fcf5c5f6685a4276796438549fbbacb`
- Repository license: Apache License 2.0
- OAI versions stated by QUEST: UE/gNB `2024.w09`, core network `2024.w04`

The CSV has no header. Column order was verified against
`tools/OAI/gNB/004-gNB-modifications.patch` at the revision above:

1. frame
2. slot
3. UE RNTI (hexadecimal text)
4. uplink SINR (dB)
5. transport block size (bytes)
6. MCS index
7. allocated PRBs
8. pending uplink buffer bytes (`estimated_ul_buffer - sched_ul_bytes`, clamped at zero)
9. head-of-line delay (ms)
10. measured uplink delay (ms)

This repository preserves the original file unchanged. The source documentation does
not establish that this specific file was captured over a USRP radio link, so LiteCore
labels it as public OAI experimental data rather than USRP over-the-air measurements.
