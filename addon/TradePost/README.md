# TradePost (addon)

Phase 1. Not implemented yet.

Planned modules:

```
TradePost.toc
Core.lua          init, events, namespace
Market.lua        region / realm / house detection
Scanner.lua       ReplicateItems harvest or scan, aggregation
LocalDB.lua       own scans, pruning
Central.lua       loader for TradePost_Data
Tooltips.lua      TooltipDataProcessor + fallbacks
Auctionator.lua   read-only display bridge (optional)
Export.lua        SavedVariables export buffer
Commands.lua      /tradepost ...
UI/Options.lua    configuration
```

Target: `## Interface: 16001`, no external libraries, MIT.
