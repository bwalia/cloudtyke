# Lab: hub-and-spoke networking and spoke-to-spoke traffic

**Level:** associate / platform engineer · **Time:** 45-60 minutes · **Cost:** nothing, it runs on CloudTyke

You join Contoso's platform team. Two application teams live in separate spoke VNets, and shared services
(DNS, tooling) and the firewall live in a hub. The payments team (spoke A) now needs to call an API owned by the
storefront team (spoke B). Your job: make that work, explain *why* each step is needed, and keep it secure.

```
                    ┌──────────── vnet-hub 10.0.0.0/16 ─────────────┐
                    │ AzureFirewallSubnet 10.0.0.0/26  (fw 10.0.0.4) │
                    │ snet-shared         10.0.1.0/24  (10.0.1.4)    │
                    └───────▲───────────────────────────────▲────────┘
                    peering │                               │ peering
  ┌── vnet-spoke-a 10.10.0.0/16 ──┐               ┌── vnet-spoke-b 10.20.0.0/16 ──┐
  │ snet-app  10.10.1.0/24 (.4)   │    ? ? ? ?    │ snet-app  10.20.1.0/24 (.4)   │
  │ snet-data 10.10.2.0/24        │ ◄───────────► │ snet-data 10.20.2.0/24        │
  └───────────────────────────────┘               └───────────────────────────────┘
```

## What you will learn

1. What a VNet's **system routes** are, and why traffic to an unconnected private network is dropped.
2. Why a peering needs **both halves** before any traffic flows (`Initiated` vs `Connected`).
3. Why **peering is not transitive**, so a hub-and-spoke topology does not connect spokes to each other.
4. The two real-world fixes, and when to choose each:
   - **direct spoke-to-spoke peering** (simple, but `n·(n-1)/2` peerings and no inspection);
   - **routing through the hub firewall** with user-defined routes (central inspection and logging).
5. Why the firewall path also needs **`allowForwardedTraffic`** and **symmetric routes** on both spokes.
6. How **NSGs** still have the last word, and how the `VirtualNetwork` service tag behaves.

## How CloudTyke shows you what happens

CloudTyke has no real VMs, but it models Azure's routing decisions. The `.4` addresses above stand in for VMs.

| Tool | What it answers |
|---|---|
| `./verify.sh <stage> [-v]` | grades the stage: traces each expected flow, prints PASS / FAIL; `-v` shows every hop |
| `curl -s https://management.cloudtyke.local/_cloudtyke/network/trace?from=10.10.1.4&to=10.20.1.4&port=443 \| jq` | a hop-by-hop path: route chosen, peering checks, NSG rule hit, final result |
| `curl -s 'https://management.cloudtyke.local/_cloudtyke/network/routes?ip=10.10.1.4' \| jq` | the subnet's **effective routes**, the same table as *Effective routes* in the Azure portal |
| CloudTyke console → **Network** | the live topology (green = Connected, amber = Initiated) and the same trace with a visual path |

## Setup

```bash
cd azure-emulator
make up          # emulator
make shell       # toolbox with tofu, terraform, kubectl, jq
cd labs/hub-spoke
tofu init
```

## Task 1: three isolated VNets

```bash
tofu apply -var connectivity=none
./verify.sh none -v
```

Look at the effective routes of spoke A's app subnet:

```bash
curl -s 'https://management.cloudtyke.local/_cloudtyke/network/routes?ip=10.10.1.4' | jq -r '.value[] | "\(.state)\t\(.addressPrefix)\t\(.nextHopType)"'
```

You will see `10.10.0.0/16 VnetLocal`, `0.0.0.0/0 Internet`, and `10.0.0.0/8 None`, `172.16.0.0/12 None`,
`192.168.0.0/16 None`, `100.64.0.0/10 None`.

> **Why is spoke A → hub dropped?** `10.0.1.4` does not match the VNet's own range, so the longest match is Azure's
> built-in `10.0.0.0/8 → None` route: private traffic with nowhere to go is dropped, not sent to the internet.
> Traffic to `8.8.8.8` only matches `0.0.0.0/0` and leaves through the default internet route.

## Task 2: hub-and-spoke

```bash
tofu apply -var connectivity=hub-only
./verify.sh hub-only -v
```

Open the console's **Network** page: the hub sits in the middle with two green (Connected) links.

> **Try it:** create only one half of a peering (for example comment out `hub_to_spoke`, apply) and trace spoke A →
> hub. The peering is `Initiated`, no route is programmed, and the packet is still dropped.

> **Why can't spoke A reach spoke B?** The peering adds a `VNetPeering` route for the **hub's** address space only.
> Azure never copies the hub's peerings into spoke A's route table, so `10.20.1.4` still hits `10.0.0.0/8 → None`.
> This is what "peering is not transitive" means.

## Task 3: fix 1, direct spoke-to-spoke peering

```bash
tofu apply -var connectivity=direct-peering
./verify.sh direct-peering -v
```

It works, and traffic does not touch the hub. Now consider 20 spokes: that is 190 peerings, and no central place
to inspect or log east-west traffic. (Azure Virtual Network Manager can automate a mesh at that scale; see the
`avnm` lab.)

## Task 4: fix 2, through the hub firewall

```bash
tofu apply -var connectivity=hub-firewall
./verify.sh hub-firewall -v
```

Each spoke gets a route table with `10.x.0.0/16 → VirtualAppliance 10.0.0.4` for the *other* spoke. The trace now
shows: spoke A → UDR → peering to the hub → firewall forwards → peering to spoke B → delivered.

> **Why a route in both spokes?** Routing is per direction. Without the UDR in spoke B the reply would have no route
> back (or would bypass the firewall, and a stateful firewall would drop it). Check the return path with
> `./verify.sh hub-firewall -v`.

## Task 5: break it, `allowForwardedTraffic`

```bash
tofu apply -var connectivity=hub-firewall -var allow_forwarded_traffic=false
./verify.sh no-forwarding -v
```

Spoke A still reaches the hub (that traffic *originates* in spoke A), but spoke A → spoke B is dropped. The packet
arriving in spoke B was *forwarded* by the firewall, and spoke B's peering refuses forwarded traffic.

## Task 6: NSGs still decide

```bash
tofu apply -var connectivity=hub-firewall -var block_https_between_spokes=true
./verify.sh nsg -v
```

HTTPS from spoke A is denied by `deny-https-from-spoke-a` (priority 200) on spoke B's app subnet, while port 8080
is still allowed by the default `AllowVnetInBound` rule.

> **Subtle point:** spoke A is not peered with spoke B, so why does `AllowVnetInBound` match it? Azure's
> `VirtualNetwork` service tag includes the VNet, its peered VNets **and the prefixes in the subnet's user-defined
> routes**, and spoke B has a UDR for `10.10.0.0/16`.

## Clean up

```bash
tofu destroy
```

## Check your understanding (interview-style)

1. A spoke's VM can reach the hub but not another spoke. List three things you would check, in order.
2. Your firewall logs show requests from spoke A to spoke B but no replies. What is the most likely cause?
3. When would you choose direct peering over hub routing, and what do you lose?
4. What does `allowGatewayTransit` / `useRemoteGateways` do, and why does it matter for on-premises traffic?
5. You have 60 spokes in 3 regions. How would you manage peerings, routes and baseline security rules without
   hand-writing 1,770 peerings? *(Hint: Azure Virtual Network Manager.)*

## Automated grading

`test/labs/hub-spoke.sh tofu|terraform` runs every task in order, checks that there is no drift after each apply, and
grades each stage. CI runs it on every change.
