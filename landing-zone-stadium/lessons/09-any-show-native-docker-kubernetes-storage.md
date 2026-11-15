# Lesson 09: Any show: native apps, Docker, Kubernetes and storage

*About 30 minutes. Site: Chapter 6 ("How is the show built?" switch and the storage shelf), the "Studio lot" view,
and Chapter 4, "The club safe".*

## A story first: film studio cities

**Ramoji Film City** in Hyderabad, India, is called the world's largest film studio complex by Guinness World Records.
It has dozens of sound stages and permanent sets: railway stations, temples, palaces. The idea was simple. A producer
can arrive with a script and leave with a finished film, because everything is already there: stages, power, props,
editing. Crews often build sets in the open spaces, and some sets are left standing for others to reuse.

**Hengdian World Studios** in Zhejiang, China, is a huge outdoor film lot. It has a life-size copy of the Forbidden
City, used in many productions. Often around twenty films or TV dramas are shooting there at the same time.

Read these lots like a landing zone:

- the **studio lot is the platform**: gates, power, security, fire safety, canteen, prop warehouse
- each **sound stage is a landing zone**
- each **set is a workload**: built fast, used, then changed for the next production
- many productions share the lot at once, but each has its own stage and its own crew (isolation between landing
  zones)

## Why it matters

The landing zone does not care what kind of show you run. The gates, seats, security and rules are the same. But
**you** must choose how to build your set, and where to keep the things that must last. Choose badly and you lose the
film when the set comes down.

## How it works

Three ways to build a set, plus storage:

| Film studio | Workload type | Azure | AWS | GCP |
|---|---|---|---|---|
| A set built by hand on the stage: strong, but slow to change | native binary app on a VM | Virtual Machines, VM Scale Sets | EC2, Auto Scaling groups | Compute Engine, managed instance groups |
| A flat-pack set piece made in the workshop, the same on every stage | Docker container | Container Apps, App Service for containers | ECS with Fargate, App Runner | Cloud Run |
| The floor manager running many flat-pack pieces and crews on many stages | Kubernetes | AKS | EKS | GKE |
| The prop warehouse and film vault | object storage (photos, video, replays, backups) | Blob Storage | S3 | Cloud Storage |
| The shared script library everyone can open | file shares | Azure Files | EFS | Filestore |
| Each set's own lock-up cupboard | disks and Kubernetes persistent volumes | Managed Disks, PVCs on AKS | EBS, PVCs on EKS | Persistent Disk, PVCs on GKE |
| The continuity notes and score books | databases | PostgreSQL flexible server, Cosmos DB | RDS, DynamoDB | Cloud SQL, Firestore |

**Stateless and stateful.** Actors and flat-pack sets come and go. The film vault must stay. Pods and containers can be
replaced at any time. Data lives in storage that outlasts them. If your app keeps its only copy of something inside a
container, it is gone when the container is replaced.

**Pick per event.** Cricket might run its scoreboard as a native app on a VM and keep years of stats in a database.
Football might run the ticket app on Kubernetes. A concert might run a simple Docker container plus object storage
for media.

**The club safe.** Every stadium has a strong room. Only the right person opens each locker.

| Stadium | Cloud | Azure | AWS | GCP |
|---|---|---|---|---|
| The strong room | the vault service | Key Vault | Secrets Manager + KMS | Secret Manager + Cloud KMS |
| Players' wallets in their own lockers | app secrets | Key Vault secrets | Secrets Manager secrets | Secret Manager secrets |
| The key to the trophy cabinet | encryption keys | Key Vault keys / Managed HSM | KMS keys | Cloud KMS keys |
| The club's official seal | certificates | Key Vault certificates | ACM | Certificate Manager |
| A wristband, not a key on a string | workload identity | managed identity + Azure RBAC | IAM role + key policy | service account + IAM |
| A key taped to the door | a password in code or a settings file (never do this) | | | |
| "Even by mistake, the trophies can't be thrown away" | soft delete and purge protection | purge protection | recovery window, key deletion wait | secret versions, key destroy delay |
| The safe's visitor book | access logs to the control room | diagnostic logs | CloudTrail | Cloud Audit Logs |

The strong room is never rebuilt. What is inside changes per event: each event gets its own lockers, and each app
opens only its own.

**Same rules for storage.** The safety code from Lesson 04 covers storage too: no public access, encryption in
transit (TLS 1.2), no shared keys, UK only, private access through the gates.

## Do it

1. Make sure the stadium is up: `bash landing-zone-stadium/scripts/up.sh`.
2. Open `terraform/pitch.tf`. For each kit, find its storage and say which shelf it is on:
   - football: highlight clips kept a short time (the vault, with a short life)
   - cricket: PostgreSQL scorecards (the score book) and ball-tracking replays that move to cool, then archive
   - concert: stage media deleted after 30 days, and no database
3. Find the lifecycle rules. Which kit keeps data for years? Which throws it away fast?
4. In the dashboard (<http://localhost:4310>), open the summer (cricket) resource group. Find the PostgreSQL server.
   Note it has no public access: the score book stays inside.
5. In the site, flip "How is the show built?" between Native app, Docker and Kubernetes. Watch the pitch change and
   the storage shelf stay. Then press "Studio lot" and see the same idea as a film lot.
6. Chapter 4 of the site: find the key taped to the door and swap it for a wristband.

## Break it

Try to make a public storage bucket in an event. In a toolbox shell, get a token as in Lesson 01, then make a tagged
resource group and a storage account that allows public blobs:

```bash
S=/subscriptions/5b5c0000-0000-4000-8000-000000000004
curl -fsS -X PUT -H "Authorization: Bearer $T" -H 'content-type: application/json' \
  "$M$S/resourceGroups/rg-leaky?api-version=2024-11-01" \
  -d '{"location":"uksouth","tags":{"event":"leaky","event-kit":"concert","cost-centre":"CC-7100"}}' >/dev/null
curl -sS -X PUT -H "Authorization: Bearer $T" -H 'content-type: application/json' \
  "$M$S/resourceGroups/rg-leaky/providers/Microsoft.Storage/storageAccounts/stleakymedia01?api-version=2023-05-01" \
  -d '{"location":"uksouth","kind":"StorageV2","sku":{"name":"Standard_LRS"},
       "properties":{"allowBlobPublicAccess":true,"minimumTlsVersion":"TLS1_0"}}' | jq
```

You get `RequestDisallowedByPolicy`. The prop warehouse cannot be left open to the street. Now fix the body:
`"allowBlobPublicAccess":false`, `"minimumTlsVersion":"TLS1_2"` and `"allowSharedKeyAccess":false`. It is accepted.

Then think about a key taped to the door. A settings file says `DB_PASSWORD=summer123`. Anyone who can read the file,
the repo or the logs can open the score book. The fix: the app gets a **managed identity** (a wristband), Key Vault
uses **RBAC**, and the app is given a role to read only its own secret. No password in the code at all.

## Check yourself

1. In the film lot story, what is the platform, what is a landing zone, and what is a workload?
2. What is the difference between stateless and stateful? Give one example of each from a stadium event.
3. Your app writes uploaded photos to a folder inside its container. What goes wrong when the pod is replaced? Where
   should the photos go?
4. Why does the safety code block public blob access?
5. Why is a wristband (workload identity) better than a password in a settings file?

## In real Azure

The same services and rules: VMs and Scale Sets, Container Apps and App Service, AKS, Blob Storage with lifecycle
management (hot, cool, cold, archive), Azure Files, Managed Disks, PostgreSQL flexible server, Cosmos DB and Key Vault
with soft delete and purge protection. Many of these are modelled in CloudTyke: check
`azure-emulator/docs/coverage.md` for what is supported. Real Azure stores real bytes, bills by size and tier, and
charges to read data back from archive.

## Same idea in AWS and GCP

- **AWS**: EC2, ECS with Fargate, EKS, S3 (with lifecycle rules to Glacier), EFS, EBS, RDS and DynamoDB. Secrets
  Manager and KMS are the safe. **S3 Block Public Access** is the "no public bucket" rule.
- **GCP**: Compute Engine, Cloud Run, GKE, Cloud Storage (Nearline, Coldline, Archive), Filestore, Persistent Disk,
  Cloud SQL and Firestore. Secret Manager and Cloud KMS are the safe. The organisation policy
  `storage.publicAccessPrevention` is the "no public bucket" rule.
