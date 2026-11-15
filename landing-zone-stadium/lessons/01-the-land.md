# Lesson 01: The land

*About 15 minutes. Site: Chapter 1, "Build the stadium".*

## A story first: the Colosseum

In Rome, almost 2,000 years ago, the Colosseum hosted many kinds of show. In its first years the arena could even be
flooded for pretend sea battles.

Around AD 90 the Romans dug an underground level under the arena floor. It is called the **hypogeum**. It had lifts
and trapdoors. Crews raised fighters, animals and whole sets of scenery up into the arena, fast, from below. The crowd
never saw the machinery. They only saw the show change.

There was a price. With machinery under the floor, the arena could not be flooded any more. The sea battles ended.

Above the crowd hung a huge sun awning. Sailors from the navy worked it, because they knew ropes and sails.

Keep three ideas from this story:

- The underground machinery is the **platform**. It is built once, it is hidden, and it makes every show fast.
- Platform choices also **limit** what you can host. No more water shows.
- The sailors are a specialist **platform team**.

## Why it matters

Before you build a stadium, you need land and an owner. In the cloud, that "land" is the very top of your account.
Everything else sits on it: the network, the rules, every app. If you do not know who owns the land and how it is
divided, you cannot build anything safe on top.

## How it works

In Azure the land is your **tenant**. Inside the tenant there is one top folder called the **Tenant Root Group**.
Under it you make **management groups** (big plots of land). Inside those sit **subscriptions** (one building, one
bill). Inside a subscription sit **resource groups** and then **resources**.

CloudTyke's seed builds this land for you:

```
Tenant Root Group (7c10d7e4-0000-4000-8000-000000000001)   the land
├── ct-platform        "Platform"        the stadium shell
│   ├── sub-connectivity                  the gates
│   └── sub-management                    the control room
├── ct-landingzones    "Landing Zones"   the pitch
│   └── sub-events                        one event (added by this folder's seed.yaml)
└── ct-sandbox         "Sandbox"         the training ground
    └── sub-sandbox
```

Two simple rules:

1. **Things flow down.** A rule or a permission set high on the tree reaches everything below it.
2. **Plots keep things apart.** The training ground can have looser rules than the pitch, because it is a different
   plot.

## Do it

1. Start CloudTyke with the stadium seed (from the repo root):

   ```bash
   bash landing-zone-stadium/scripts/up.sh
   ```

2. Open a toolbox shell and get a token:

   ```bash
   cd azure-emulator && make shell
   M=https://management.cloudtyke.local
   T=$(curl -fsS https://login.cloudtyke.local/$ARM_TENANT_ID/oauth2/v2.0/token \
     -d grant_type=client_credentials -d client_id=$ARM_CLIENT_ID \
     -d client_secret=$ARM_CLIENT_SECRET -d scope=$M/.default | jq -r .access_token)
   ```

3. Read the land from the top:

   ```bash
   curl -fsS -H "Authorization: Bearer $T" \
     "$M/providers/Microsoft.Management/managementGroups/$ARM_TENANT_ID?api-version=2023-04-01&\$expand=children&\$recurse=true" \
     | jq '.properties.children[] | {name, kids: [.children[]?.properties.displayName]}'
   ```

   You should see `ct-platform`, `ct-landingzones` and `ct-sandbox`, each with its subscriptions.

4. Open the dashboard (`make dashboard`, then <http://localhost:4310>) and find the same tree on the Resources page.

5. Open the site (`landing-zone-stadium/site/index.html`) and play Chapter 1. Place the land first. Watch the badge:
   the land is **"Never rebuilt"**.

## Break it

Try to make a new management group with Terraform or a `PUT`:

```bash
curl -sS -X PUT -H "Authorization: Bearer $T" -H 'content-type: application/json' \
  "$M/providers/Microsoft.Management/managementGroups/ct-new?api-version=2023-04-01" -d '{}' | jq
```

It fails. In CloudTyke, management groups come only from the seed. The land is set before the building starts. That
is why our Terraform reads them with `data "azurerm_management_group"` and never creates them.

Ask yourself: who in a real company should be allowed to change the land? (Answer: very few people. A small platform
team, through reviewed code.)

## Check yourself

1. What is the Tenant Root Group, in stadium words?
2. Name the three management groups under it in CloudTyke.
3. A rule is set on `ct-landingzones`. Does it reach `sub-events`? Does it reach `sub-sandbox`?
4. In the Colosseum story, what was the price of building the hypogeum? What is the cloud version of that?
5. Why is the training ground a separate plot, and not just a corner of the pitch?

## In real Azure

The same. Every tenant has a Tenant Root Group, and you build management groups under it in the portal, with `az
account management-group create`, or with Terraform. Real Azure lets you create them through the API. CloudTyke keeps
them fixed in the seed so every learner sees the same tree. Moving a subscription between management groups is a big
change in real life, because the rules that flow down change with it.

## Same idea in AWS and GCP

- **AWS**: the land is your **Organization**, with one **root**. Plots are **organisational units (OUs)**. Buildings
  are **accounts**.
- **GCP**: the land is the **Organization node**. Plots are **folders**. Buildings are **projects**.

The words change. The tree, and the rule that things flow down, stay the same.
