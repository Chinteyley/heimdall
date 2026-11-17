# Heimdall console

A chat that runs through Bifrost with Heimdall's mode switch, the checked claims marked inline, a freshness panel with sources, and a "Check an answer" tab. See the [main README](../README.md) for setup.

```bash
printf 'BIFROST_URL=http://localhost:8080\nBIFROST_VK=%s\n' "$(grep ^BIFROST_VK_APP= ../.env | cut -d= -f2)" > .env.local
bun install && bun dev
```
