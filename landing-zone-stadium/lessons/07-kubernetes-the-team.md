# Lesson 07: Kubernetes is the team

*About 25 minutes. Site: Chapter 7, "The team" ("Meet the team" view).*

## Why it matters

Kubernetes lives **on the pitch**, inside one event's landing zone. The gates, seats and safety code do not care if
you run Kubernetes or not. But many events do use it, so you need a clear picture of what it does.

Fun fact: the word Kubernetes comes from Greek for "helmsman", the person who steers a ship. A football manager steers
a team. The picture fits.

## How it works

You do not tell Kubernetes "start this program on that machine". You write down **what you want** (the team sheet),
and Kubernetes keeps making it true, all match long.

| Football | Kubernetes |
|---|---|
| The manager and coaching staff | the control plane |
| The team sheet and formation written before kick-off | the manifest: the desired state |
| The manager decides who plays where | the scheduler places pods on nodes |
| Players | pods |
| Positions on the pitch | nodes |
| "We always want 11 on the pitch" | a Deployment with `replicas` |
| A player is injured, a sub comes on | self-healing: a failed pod is replaced |
| Players on the bench | spare capacity, pending pods |
| Coach on the touchline reporting back | the kubelet on each node |
| The referee who stops illegal play | admission control and policy (Azure Policy for AKS, Gatekeeper) |
| The kit man | the container registry |
| More stewards at the gates when queues grow | autoscaling |

Other events, same idea:

- **Cricket**: the captain sets the field. Placing fielders is like scheduling: put each one where they are needed.
- **Concert**: the stage manager calls the cues. Lights, sound and band come in on time, in the right place.

Where it sits in our stadium: each event's AKS cluster is in that event's resource group, on that event's seats (its
spoke subnet). Its outbound traffic goes through the hub firewall. Its images come from the shared kit room (the
Container Registry). The safety code says AKS must use Entra RBAC with local accounts off: the dressing room opens with
stadium wristbands, not a spare key.

## Do it

You need real pods for this, so run AKS on k3s. From the repo root:

```bash
cd azure-emulator && make up-k3s && cd ..
bash landing-zone-stadium/scripts/up.sh
```

Then, in a toolbox shell (`cd azure-emulator && make shell`), sign in to the derby cluster. The safety code turns
local accounts off, so there is no admin password. You sign in with Entra ID, like a pipeline does (this is what
`kubelogin` does for you in real Azure). The resource group and cluster names come from the Terraform outputs
(`up.sh` prints them):

```bash
M=https://management.cloudtyke.local
tok() { curl -fsS https://login.cloudtyke.local/$ARM_TENANT_ID/oauth2/v2.0/token \
  -d grant_type=client_credentials -d client_id=$ARM_CLIENT_ID \
  --data-urlencode client_secret=$ARM_CLIENT_SECRET -d scope="$1" | jq -r .access_token; }
T=$(tok "$M/.default")                                   # a wristband for Azure
K=$(tok 6dae42f8-4368-4678-94ff-3960e28e3630/.default)    # a wristband for the AKS API server

S=/subscriptions/5b5c0000-0000-4000-8000-000000000004
RG=<derby resource group>      # from the outputs
AKS=<derby AKS cluster name>   # from the outputs
kc=$(curl -fsS -X POST -H "Authorization: Bearer $T" \
  "$M$S/resourceGroups/$RG/providers/Microsoft.ContainerService/managedClusters/$AKS/listClusterUserCredential?api-version=2025-02-01" \
  | jq -r '.kubeconfigs[0].value' | base64 -d)
server=$(awk '/server:/ {print $2; exit}' <<<"$kc")
ca=$(awk '/certificate-authority-data:/ {print $2; exit}' <<<"$kc")
cat > /tmp/derby.kc <<CFG
apiVersion: v1
kind: Config
clusters: [{ name: derby, cluster: { server: "$server", certificate-authority-data: "$ca" } }]
users: [{ name: me, user: { token: "$K" } }]
contexts: [{ name: derby, context: { cluster: derby, user: me } }]
current-context: derby
CFG
export KUBECONFIG=/tmp/derby.kc
until kubectl get --raw /readyz >/dev/null 2>&1; do sleep 3; done   # the k3s node starts on first use
kubectl get nodes
```

If `kubectl` says "forbidden", your principal needs a Kubernetes role on the cluster, for example **Azure Kubernetes
Service RBAC Cluster Admin**. That is Azure RBAC for Kubernetes doing its job: the steward checks your wristband.

Now pick a team:

```bash
kubectl create namespace tickets
kubectl -n tickets create deployment turnstile --image=nginx:alpine --replicas=11
kubectl -n tickets get pods -o wide        # 11 players, and the node each one plays on
```

Injure a player and watch the sub come on:

```bash
kubectl -n tickets delete pod "$(kubectl -n tickets get pods -o name | head -1 | cut -d/ -f2)"
kubectl -n tickets get pods               # still 11: a new pod replaced the deleted one
```

Change the team sheet:

```bash
kubectl -n tickets scale deployment turnstile --replicas=15
kubectl -n tickets get deployment turnstile
```

In the site, Chapter 7, click the pitch, then "Kubernetes". The formation drawing shows each label. Switch the venue
to cricket or concert to see the captain or the stage manager.

## Break it

1. **A player who cannot get a kit.** Create a deployment with an image that does not exist:

   ```bash
   kubectl -n tickets create deployment ghost --image=nginx:does-not-exist
   kubectl -n tickets get pods
   ```

   The pod sits in `ErrImagePull` / `ImagePullBackOff`. The kit man has nothing to hand over. Read the events with
   `kubectl -n tickets describe pod -l app=ghost`. Delete it after.

2. **Too many players for the pitch.** Scale to 500 replicas. Some pods stay `Pending`: they are on the bench because no
   node has room. In Lesson 08 you learn how to size them and when the stadium opens the upper tier (more nodes).
   Scale back to 11.

## Check yourself

1. What is the control plane, in football words?
2. You delete a pod from a Deployment with `replicas: 11`. What happens, and why?
3. What does a `Pending` pod mean?
4. Who is the referee, and why does a landing zone care about it?
5. Where does Kubernetes live: on the stadium shell or on the pitch? Why does that matter?

## In real Azure

In AKS, Azure runs the control plane for you. You run the nodes, in node pools, inside your own subnet. **Azure Policy
for AKS** (built on Gatekeeper) is the referee inside the cluster. **Microsoft Entra ID** and **Azure RBAC for
Kubernetes** decide who may use `kubectl`. **Workload identity** lets pods use Azure without passwords (see Lesson 09
and Chapter 4 of the site). CloudTyke models the AKS API and, with `make up-k3s`, backs each cluster with a real k3s
container so `kubectl` really works.

## Same idea in AWS and GCP

- **AWS**: **EKS** runs the control plane. Nodes are EC2 instances in managed node groups, or Fargate. The kit man is
  **ECR**.
- **GCP**: **GKE** runs the control plane. GKE Autopilot even runs the nodes for you. The kit man is **Artifact
  Registry**.

Kubernetes itself is the same everywhere. The team sheet you wrote here runs on all three.
