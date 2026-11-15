# Lab: Azure Virtual Network Manager, networking at scale across regions

**Level:** platform / network engineer · **Time:** 60 minutes · **Prerequisite:** the [hub-and-spoke lab](../hub-spoke)

Contoso now runs in **UK South** and **West Europe**, with a hub per region and more spokes every month. The
hand-written peerings and route tables from the hub-and-spoke lab do not scale: 20 spokes in a full mesh would need
190 peerings, and every new spoke would mean editing Terraform in several places. In this lab you hand connectivity
and baseline security to **Azure Virtual Network Manager (AVNM)**.

```
   UK South                                      West Europe
   ┌───────────────┐                             ┌────────────────┐
   │ vnet-hub-uks  │ 10.0.0.0/16                 │ vnet-hub-weu   │ 10.100.0.0/16
   │  snet-shared  │ 10.0.1.0/24                 │  snet-shared   │ 10.100.1.0/24
   │  AzureBastion │ 10.0.2.0/26                 │  AzureBastion  │ 10.100.2.0/26
   └──┬────────┬───┘                             └──┬─────────┬───┘
      │        │   ANM_ peerings (by AVNM)          │         │
  ┌───┴──┐  ┌──┴───┐                            ┌───┴──┐  ┌───┴──┐
  │uks-1 │  │uks-2 │ 10.1/16, 10.2/16 …         │weu-1 │  │weu-2 │ 10.101/16, 10.102/16 …
  └──────┘  └──────┘                            └──────┘  └──────┘
```

## Concepts

| AVNM concept | What it is | In this lab |
|---|---|---|
| **Network manager** | the control plane, scoped to subscriptions or management groups | `avnm-contoso`, scoped to the sandbox subscription |
| **Network group** | a set of VNets (static members, or dynamic via Azure Policy) | `ng-spokes-uks`, `ng-spokes-weu`, `ng-spokes-global` |
| **Connectivity configuration** | the topology to build: `HubAndSpoke` or `Mesh` | one hub-and-spoke per region, one global mesh |
| **Connected group** | members reach each other directly, **without peering resources** (`ConnectedGroup` routes) | `DirectlyConnected` spokes, the global mesh |
| **Security admin rules** | organisation rules evaluated **before NSGs**: `Deny`, `Allow` (NSG decides) or `AlwaysAllow` (skips NSGs) | deny SSH/RDP everywhere, always allow it from bastions |
| **Deployment (commit)** | nothing happens until you commit configurations **to a region**; a commit is a snapshot | one connectivity and one security deployment per region |

There is not a single `azurerm_virtual_network_peering` or route table in [main.tf](main.tf).

## Setup

```bash
cd azure-emulator && make up && make shell
cd labs/avnm && tofu init
```

## Task 1: explore before deploying

```bash
tofu apply -var stage=hub-spoke -target=azurerm_network_manager_connectivity_configuration.hub_spoke
curl -s 'https://management.cloudtyke.local/_cloudtyke/network/trace?from=10.1.1.4&to=10.0.1.4' | jq -r .summary
```

The configurations exist, but the spoke still cannot reach its hub. **Configurations are inert until committed.**

## Task 2: hub-and-spoke in every region

```bash
tofu apply -var stage=hub-spoke
./verify.sh hub-spoke -v
```

- `verify.sh` first prints `listDeploymentStatus`: one **Connectivity** deployment per region.
- List a spoke's peerings: the names start with `ANM_`. AVNM created and owns them.
  ```bash
  curl -s 'https://management.cloudtyke.local/_cloudtyke/network/routes?ip=10.1.1.4' | jq -r '.value[] | "\(.addressPrefix)\t\(.nextHopType)\t\(.name // "")"'
  ```
- Spoke-to-spoke traffic is still dropped: hub-and-spoke on its own does not connect spokes, just like the
  previous lab.

## Task 3: connect spokes within a region

```bash
tofu apply -var stage=regional-mesh
./verify.sh regional-mesh -v
```

`group_connectivity = "DirectlyConnected"` puts each region's spokes in a **connected group**. The effective routes
now show `ConnectedGroup` next hops, with no new peering resources. Traffic between regions is still dropped.

> **Why does the deployment have `triggers`?** A commit is a snapshot. Editing a deployed configuration changes
> nothing until it is committed again. Remove `triggers`, change `stage`, and watch the traces stay the same until
> you taint the deployment. This catches people out in production.

## Task 4: global mesh

```bash
tofu apply -var stage=global-mesh
./verify.sh global-mesh -v
```

`cc-global-mesh` (Mesh, `global_mesh_enabled = true`) is committed to **both** regions, and every spoke reaches every
spoke across regions. Notice what still fails: a spoke cannot reach the *other region's hub*. Hubs are not members of
the mesh. Is that what you want? (Many designs mesh the hubs instead and route through regional firewalls.)

## Task 5: guardrails with security admin rules

```bash
tofu apply -var stage=global-mesh -var security_admin=true
./verify.sh security -v
```

| Flow | Result | Why |
|---|---|---|
| spoke → spoke SSH / RDP | **Blocked** | `deny-ssh-rdp` (Deny, priority 100), before any NSG |
| spoke → spoke HTTPS | Delivered | no admin rule matches; NSGs decide |
| UK bastion → uks spoke 1 SSH | **Delivered** | `always-allow-bastion` (AlwaysAllow, priority 10) wins over the team NSG's `team-denies-ssh` |

In Task 4 the same bastion flow was **Blocked** by the team's NSG. An application team cannot lock the platform out,
and cannot open SSH to the world either.

## Task 6: scale it

```bash
tofu apply -var stage=global-mesh -var security_admin=true -var spokes_per_region=10
tofu output managed_vnets   # 22 VNets
```

Ten new spokes per region join the groups and the deployed configurations with no other change. Trace any pair with
the `curl` above.

## Break it (optional)

- **Overlapping address spaces:** change one West Europe spoke to `10.1.0.0/16` and apply with `stage=global-mesh`.
  The connected group cannot contain overlapping ranges, so the deployment status for that region turns **Failed**
  with the two VNets named.
- **Out of scope:** add a VNet from another subscription to a group. The network manager refuses it
  (`ResourceNotInNetworkManagerScope`) until that subscription is in its scope.
- **Delete a configuration that is deployed:** `ConfigurationIsDeployed`. Remove it from the deployment first.

## Clean up

```bash
tofu destroy
```

## Check your understanding

1. What is the difference between hub-and-spoke with `DirectlyConnected` and a Mesh configuration?
2. A new spoke was added to a network group but cannot reach anything. What do you check first?
3. Security admin `Allow` vs `AlwaysAllow`: when would you use each?
4. Why can a team NSG not override an admin `Deny`, and why is that useful in a regulated environment?
5. You need spoke traffic inspected by a regional firewall. Which AVNM feature, or other Azure feature, would you
   combine with hub-and-spoke? *(Hint: routing configurations / UDRs to the hub firewall.)*

## Automated grading

`test/labs/avnm.sh tofu|terraform` applies every stage, checks there is no drift, grades each one and destroys.
