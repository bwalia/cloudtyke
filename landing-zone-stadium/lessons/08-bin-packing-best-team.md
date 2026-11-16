# Lesson 08: Bin packing, the best team

*About 30 minutes. Site: Chapter 7, "The team" ("Pack the squad" and "Seat the crowd").*

## Why it matters

A good manager gets the most from the squad without wasting anyone. A good stadium fills the lower tier before it
opens the upper tier, so fewer stewards and cleaners are needed.

Kubernetes does the same with **bin packing**: it fits pods onto as few nodes as safely possible, so you pay for fewer
machines. But packing too tight is risky. If one node fails and it held all your important pods, the match is lost.
This lesson is about finding the right balance.

## How it works

| Football / stadium | Kubernetes | Why it matters |
|---|---|---|
| Each player's role and how much running they need | resource `requests` and `limits` (CPU, memory) | the scheduler can only pack well if every pod says what it needs |
| Fitness coach measures what each player really needs | right-sizing (Vertical Pod Autoscaler recommendations) | asking for too much wastes space, too little causes slowdowns and evictions |
| Fill one stand before opening the next | bin packing: the scheduler's `MostAllocated` scoring, or Karpenter consolidation (AKS Node Auto Provisioning, EKS Karpenter, GKE "optimize-utilization") | fewer nodes, lower cost |
| Spread fans across all stands | spreading: the default `LeastAllocated` style | more headroom, but more nodes |
| Don't put both centre-backs on one side | pod anti-affinity, topology spread across zones | one injury (one node or zone failure) must not lose the whole defence |
| Only the goalkeeper plays in goal | taints and tolerations, node pools (for example GPU or memory pools) | special nodes only for pods that need them |
| Keep the striker near the playmaker | pod affinity | low latency between parts that talk a lot |
| The star player always gets picked | `PriorityClass` and preemption | key apps win when space is short |
| "At least 10 must stay on the pitch" | `PodDisruptionBudget` | safe upgrades and repacking without losing the match |
| Half-time tactical reshuffle | descheduler / consolidation moves pods to pack them again | the pitch stays tidy as the game changes |
| More stewards at the gates when queues grow | Horizontal Pod Autoscaler, then cluster autoscaler adds nodes | handle the kick-off rush, then shrink after |

**Requests and limits.** A **request** is what the pod books: "I need half a CPU and 256 MiB". The scheduler uses
requests, not real use, to decide where a pod fits. A **limit** is the most it may use. Too much CPU and it is slowed
down. Too much memory and it is stopped.

**Seat plans are bin packing too.** A concert stage blocks the seats behind it, and the floor becomes standing room.
Same seats, different shape. Single empty seats nobody can buy ("orphan seats") are like **fragmentation**: spare CPU
on one node, spare memory on another, but no node can fit the next pod. A family of five must sit together (pod
affinity, or gang scheduling). Away fans get their own block with a buffer (namespaces, network policies, separate
node pools). Some seats are held back for stewards (`kube-reserved` for system pods). The upper tier opens only when
the lower tier is full (the cluster autoscaler adds a node only when pods cannot fit). An empty stand is closed after
the crowd leaves (scale down and consolidate).

## Do it

This needs real pods. Start CloudTyke with k3s and the stadium, then sign in to the derby cluster exactly as in
Lesson 07 (`export KUBECONFIG=/tmp/derby.kc`).

> The k3s backend runs **one node** per cluster. That is enough to see requests, limits, spread rules and budgets
> work. In real AKS with three zones you would also see pods land in different zones.

1. **How big is the pitch?**

   ```bash
   kubectl get nodes -L topology.kubernetes.io/zone
   kubectl describe node | sed -n '/Allocatable:/,/System Info:/p'
   ```

   Note the allocatable CPU and memory. That is the space you can pack.

2. **A team with a proper team sheet.** Save this as `squad.yaml`:

   ```yaml
   apiVersion: apps/v1
   kind: Deployment
   metadata: { name: fan-app, namespace: tickets }
   spec:
     replicas: 6
     selector: { matchLabels: { app: fan-app } }
     template:
       metadata: { labels: { app: fan-app } }
       spec:
         topologySpreadConstraints:
           - maxSkew: 1
             topologyKey: kubernetes.io/hostname
             whenUnsatisfiable: ScheduleAnyway
             labelSelector: { matchLabels: { app: fan-app } }
         containers:
           - name: web
             image: nginx:alpine
             resources:
               requests: { cpu: 100m, memory: 64Mi }
               limits:   { cpu: 250m, memory: 128Mi }
   ---
   apiVersion: policy/v1
   kind: PodDisruptionBudget
   metadata: { name: fan-app, namespace: tickets }
   spec:
     minAvailable: 5
     selector: { matchLabels: { app: fan-app } }
   ```

   Apply it and look:

   ```bash
   kubectl create namespace tickets --dry-run=client -o yaml | kubectl apply -f -
   kubectl apply -f squad.yaml
   kubectl -n tickets get pods -o wide
   kubectl describe node | sed -n '/Allocated resources:/,/Events:/p'
   kubectl -n tickets get pdb fan-app
   ```

   The node now shows `600m` of CPU requested (6 pods times 100m). The budget says only **one** pod may be taken
   away at a time (`ALLOWED DISRUPTIONS 1`). "At least 5 must stay on the pitch."

3. **Spread across zones.** Change the `topologyKey` to `topology.kubernetes.io/zone` and `whenUnsatisfiable` to
   `DoNotSchedule`, then apply again. In real AKS with zones 1, 2 and 3, the pods land evenly across them. Here, look
   at the node's labels from step 1. If it has no zone label, new pods stay `Pending`: the rule cannot be met. Read why
   with `kubectl -n tickets describe pod <a pending pod>`. Put it back to `kubernetes.io/hostname`.

4. **Fill the pitch.** Raise the requests until they do not fit:

   ```bash
   kubectl -n tickets set resources deployment fan-app --requests=cpu=1,memory=64Mi --limits=cpu=1,memory=128Mi
   kubectl -n tickets get pods
   ```

   Some pods stay `Pending` with "Insufficient cpu". On a real AKS pool with autoscaling, this is the moment the
   cluster autoscaler opens the upper tier: it adds a node.

5. In the site, Chapter 7, play **Pack the squad**. Try "Auto: Pack" and "Auto: Spread", then press "Zone failure".
   Then play **Seat the crowd** and count the orphan seats.

## Break it

Remove the requests and limits:

```bash
kubectl -n tickets patch deployment fan-app --type=json \
  -p='[{"op":"remove","path":"/spec/template/spec/containers/0/resources"}]'
kubectl -n tickets scale deployment fan-app --replicas=40
kubectl describe node | sed -n '/Allocated resources:/,/Events:/p'
```

All 40 pods are placed, and the node says almost nothing is requested. The scheduler packs them blindly, because no
pod said what it needs. On a busy match day they would fight over CPU and memory, and the node could start to evict
them. The autoscaler would never add a node, because on paper the pitch is empty.

Now try to break the budget. Set the PDB to `minAvailable: 40` while you have 40 replicas, then:

```bash
kubectl -n tickets get pdb fan-app      # ALLOWED DISRUPTIONS 0
```

With zero allowed disruptions, a node upgrade or a repack can never move a pod. Too strict a budget blocks safe
maintenance. Put it back to a sensible number, then clean up with `kubectl delete namespace tickets`.

## Check yourself

1. Does the scheduler place pods by what they really use, or by what they request?
2. What goes wrong if requests are far too high? And far too low?
3. Bin packing saves money. What is the risk, and which two settings reduce it?
4. What is an "orphan seat" in Kubernetes words?
5. When does the cluster autoscaler add a node?

## In real Azure

AKS node pools can span **availability zones** 1, 2 and 3, and nodes carry the `topology.kubernetes.io/zone` label.
The **cluster autoscaler** uses each pool's `min_count` and `max_count` (the football kit's pool goes from 3 to 30).
**Node Auto Provisioning** (based on Karpenter) can pick node sizes and consolidate for you. **Azure Monitor
Container insights** shows real use next to requests, which helps right-sizing. CloudTyke's k3s backend gives you a
real scheduler on one node; zones and the autoscaler are modelled in the AKS API, not run.

## Same idea in AWS and GCP

- **AWS**: EKS uses the same Kubernetes settings. **Karpenter** picks nodes and consolidates them to pack tighter.
- **GCP**: GKE has an autoscaling profile called **optimize-utilization** that packs more tightly and scales down
  faster. **GKE Autopilot** charges by pod requests, so honest requests save money directly.
