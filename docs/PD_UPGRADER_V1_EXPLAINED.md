# PD Upgrader in Tikv-Operator V1 - Explanation

## Overview

The `pkg/manager/member/pd_upgrader.go` file in tikv-operator v1 handled **rolling upgrades of PD (Placement Driver) instances** in a StatefulSet-based architecture. This was a critical component for safely upgrading PD clusters while maintaining quorum and avoiding service disruption.

## Key Responsibilities

### 1. **StatefulSet Partition Management**
- Controlled the `spec.updateStrategy.rollingUpdate.partition` field
- Used partition to control which Pods get updated (only Pods with ordinal >= partition are updated)
- Incrementally lowered the partition to upgrade PD instances one at a time

### 2. **Leader Transfer Coordination**
- **Critical for PD**: PD uses a Raft consensus protocol with a leader
- Before upgrading the PD leader, the upgrader coordinated leader transfer
- Ensured a new leader was elected before upgrading the old leader
- Prevented quorum loss during upgrades

### 3. **Quorum Maintenance**
- Ensured at least (N+1)/2 PD members remained available (quorum)
- For a 3-member cluster: at least 2 members must be healthy
- For a 5-member cluster: at least 3 members must be healthy
- Blocked upgrades if quorum would be lost

### 4. **Pod Deletion Control**
- Controlled when to delete Pods for recreation with new image
- Used StatefulSet partition to prevent premature Pod deletion
- Ensured only one PD instance was upgraded at a time

## How It Worked

### Upgrade Flow

```go
func (tku *pdUpgrader) Upgrade(tc *v1alpha1.TikvCluster, oldSet *apps.StatefulSet, newSet *apps.StatefulSet) error {
    // 1. Check if PD is in upgrade phase
    if tc.Status.PD.Phase == v1alpha1.UpgradePhase {
        // Prevent upgrades if PD is already upgrading
        return nil
    }

    // 2. Set upgrade phase
    tc.Status.PD.Phase = v1alpha1.UpgradePhase

    // 3. Check if upgrade is needed (template changed)
    if !templateEqual(newSet, oldSet) {
        return nil
    }

    // 4. Check if all Pods are already upgraded
    if oldSet.Spec.UpdateStrategy.RollingUpdate.Partition == nil ||
       *oldSet.Spec.UpdateStrategy.RollingUpdate.Partition == 0 {
        return nil // Already upgraded
    }

    // 5. Get current partition
    partition := *oldSet.Spec.UpdateStrategy.RollingUpdate.Partition

    // 6. Get Pod ordinal to upgrade (highest ordinal first, reverse order)
    podOrdinals := helper.GetPodOrdinals(*oldSet.Spec.Replicas, oldSet).List()
    for i := len(podOrdinals) - 1; i >= 0; i-- {
        ordinal := podOrdinals[i]
        if ordinal < partition {
            continue // Already upgraded
        }

        // 7. Upgrade this specific PD Pod
        return tku.upgradePDPod(tc, ordinal, newSet)
    }

    return nil
}
```

### Individual Pod Upgrade

```go
func (tku *pdUpgrader) upgradePDPod(tc *v1alpha1.TikvCluster, ordinal int32, newSet *apps.StatefulSet) error {
    podName := PDPodName(tcName, ordinal)
    
    // 1. Get PD member info from PD API
    member := tku.getMemberByOrdinal(tc, ordinal)
    if member == nil {
        return fmt.Errorf("member not found for ordinal %d", ordinal)
    }

    // 2. Check if this PD is the leader
    if member.IsLeader {
        // CRITICAL: Transfer leadership before upgrading leader
        err := tku.transferLeader(tc, member)
        if err != nil {
            return fmt.Errorf("failed to transfer leader: %v", err)
        }
        // Wait for leader transfer to complete
        return controller.RequeueErrorf("waiting for leader transfer")
    }

    // 3. Ensure PD member is healthy
    if !member.Health {
        return controller.RequeueErrorf("PD member %s is not healthy", podName)
    }

    // 4. Lower partition to allow this Pod to be updated
    setUpgradePartition(newSet, ordinal-1)
    
    return nil
}
```

### Leader Transfer

```go
func (tku *pdUpgrader) transferLeader(tc *v1alpha1.TikvCluster, leaderMember *PDMember) error {
    // 1. Get all PD members
    members := tku.getMembers(tc)
    
    // 2. Find a non-leader healthy member
    var targetMember *PDMember
    for _, m := range members {
        if m.Name != leaderMember.Name && m.Health {
            targetMember = m
            break
        }
    }
    
    if targetMember == nil {
        return fmt.Errorf("no healthy non-leader member found")
    }

    // 3. Call PD API to transfer leadership
    pdClient := tku.pdControl.GetPDClient(...)
    err := pdClient.TransferPDLeader(targetMember.Name)
    if err != nil {
        return err
    }

    // 4. Wait and verify transfer completed
    // (This would be handled in next reconciliation)
    
    return nil
}
```

### Partition Control

```go
func setUpgradePartition(newSet *apps.StatefulSet, partition int32) {
    if newSet.Spec.UpdateStrategy.RollingUpdate == nil {
        newSet.Spec.UpdateStrategy.RollingUpdate = &apps.RollingUpdateStatefulSetStrategy{}
    }
    newSet.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
}
```

## Key Concepts

### 1. **StatefulSet Partition**
- Partition value determines which Pods are "protected" from updates
- Pods with ordinal < partition keep the old template
- Pods with ordinal >= partition get the new template
- Example: Partition = 2 means Pods 0 and 1 stay on old version, Pods 2+ get new version

### 2. **Reverse Order Upgrade**
- Upgrades start from the highest ordinal (e.g., PD-2) and move down (PD-1, PD-0)
- This ensures the lowest ordinal (typically PD-0) is upgraded last
- Helps maintain stable cluster identity

### 3. **Leader Safety**
- **Never upgrade the leader directly**
- Must transfer leadership first
- Wait for transfer completion before upgrading
- Prevents quorum loss and service disruption

### 4. **Health Checks**
- Only upgrade healthy PD members
- Ensures cluster stability during upgrades
- Unhealthy members are skipped until they recover

## Example Upgrade Scenario

### Initial State
- Cluster: 3 PD members (PD-0, PD-1, PD-2)
- Current version: v5.0.0
- Target version: v6.0.0
- PD-0 is the leader

### Upgrade Steps

1. **Set partition to 3** (protect all Pods)
   ```yaml
   spec:
     updateStrategy:
       rollingUpdate:
         partition: 3
   ```

2. **Upgrade PD-2** (non-leader, highest ordinal)
   - Check: PD-2 is healthy ✓
   - Check: PD-2 is not leader ✓
   - Lower partition to 2
   - StatefulSet updates PD-2 Pod
   - Wait for PD-2 to be ready

3. **Upgrade PD-1** (non-leader)
   - Check: PD-1 is healthy ✓
   - Check: PD-1 is not leader ✓
   - Lower partition to 1
   - StatefulSet updates PD-1 Pod
   - Wait for PD-1 to be ready

4. **Upgrade PD-0** (leader)
   - Check: PD-0 is leader ⚠️
   - **Transfer leadership** from PD-0 to PD-1 (or PD-2)
   - Wait for leader transfer to complete
   - Verify new leader is elected
   - Lower partition to 0
   - StatefulSet updates PD-0 Pod
   - Wait for PD-0 to be ready

5. **Complete**
   - All PD members upgraded to v6.0.0
   - Cluster stable and healthy

## Why This Approach Was Needed

### StatefulSet Limitations
- StatefulSet doesn't support pre-upgrade hooks
- No built-in way to coordinate with application state (like PD leader status)
- Partition control requires manual manipulation

### PD-Specific Requirements
- **Quorum critical**: Losing quorum means cluster becomes unavailable
- **Leader critical**: Leader handles all writes; must be available
- **Raft consensus**: Members must coordinate during membership changes

## Migration to V2

In tikv-operator v2:
- **No StatefulSet**: Direct Pod management
- **Task-based pattern**: Upgrade logic broken into composable tasks
- **PD Instance Controller**: Each PD instance managed individually
- **PDGroup Controller**: Coordinates upgrades across instances
- **More flexible**: Can handle complex upgrade scenarios better

The v1 `pd_upgrader.go` logic is replaced by:
- `pkg/controllers/pdgroup/tasks/updater.go` - Coordinates instance updates
- `pkg/controllers/pd/tasks/pod.go` - Handles individual Pod lifecycle
- `pkg/controllers/pd/tasks/status.go` - Manages PD member status

## Summary

The `pd_upgrader.go` in v1 was responsible for:
1. ✅ Safely upgrading PD instances one at a time
2. ✅ Coordinating leader transfer before upgrading the leader
3. ✅ Maintaining quorum during upgrades
4. ✅ Controlling StatefulSet partition to manage upgrade order
5. ✅ Ensuring only healthy members are upgraded

This was critical for maintaining PD cluster availability during upgrades, as PD is essential for TiKV cluster operation (manages metadata, scheduling, and coordination).

