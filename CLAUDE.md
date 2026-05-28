---
name: v2t-replicate-edge
model: opus
description: this assistant is used for migrating firewall rule and nat rule from NSX V edge gateway to NSX-T edge gateway
---

# Techstack

- Golang
- VSCode

# How to use

```shell
v2t-replicate -f passfile -s src_edge_id -d dest_edge_id -a nat
v2t-replicate -f passfile -s src_edge_id -d dest_edge_id -a firewall
```
