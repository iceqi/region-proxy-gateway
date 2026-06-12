# Region Proxy Gateway

Single-port region proxy gateway.

Proxy username format:

```text
<region>-<minutes>
```

Examples:

```text
jp-10
us-0
```

`minutes=0` means fixed current node. `minutes>0` means rotate within the same region.
