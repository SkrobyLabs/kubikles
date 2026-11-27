import { describe, expect, it } from 'vitest';
import { extractControllerOwnerFromYaml } from './k8s-helpers';

// Characterization of the text-based controller owner extraction. The implementation
// is regex based and is about to be optimised for large documents, so both the
// intended behaviour and a few deliberate quirks are pinned.

const podYaml = (owners: string, tail = '') => `apiVersion: v1
kind: Pod
metadata:
  creationTimestamp: "2024-01-01T00:00:00Z"
  name: web-abc-123
  namespace: default
${owners}
  resourceVersion: "42"
  uid: 11111111-2222-3333-4444-555555555555
spec:
  containers:
  - image: nginx
    name: nginx
${tail}`;

const replicaSetOwner = `  ownerReferences:
  - apiVersion: apps/v1
    blockOwnerDeletion: true
    controller: true
    kind: ReplicaSet
    name: web-abc
    uid: aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee`;

describe('extractControllerOwnerFromYaml', () => {
    it('returns null for empty input', () => {
        expect(extractControllerOwnerFromYaml('')).toBeNull();
        expect(extractControllerOwnerFromYaml(null)).toBeNull();
        expect(extractControllerOwnerFromYaml(undefined)).toBeNull();
    });

    it('returns null when there is no ownerReferences block', () => {
        expect(extractControllerOwnerFromYaml(podYaml(''))).toBeNull();
    });

    it('extracts kind, name and uid of the controller owner', () => {
        expect(extractControllerOwnerFromYaml(podYaml(replicaSetOwner))).toEqual({
            kind: 'ReplicaSet', name: 'web-abc', uid: 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee',
        });
    });

    it('selects the controller among several owners regardless of order', () => {
        const owners = `  ownerReferences:
  - apiVersion: v1
    kind: ConfigMap
    name: not-controller
    uid: u-1
  - apiVersion: apps/v1
    controller: true
    kind: StatefulSet
    name: db
    uid: u-2`;
        expect(extractControllerOwnerFromYaml(podYaml(owners))).toEqual({ kind: 'StatefulSet', name: 'db', uid: 'u-2' });
        const reversed = `  ownerReferences:
  - apiVersion: apps/v1
    controller: true
    kind: StatefulSet
    name: db
    uid: u-2
  - apiVersion: v1
    kind: ConfigMap
    name: not-controller
    uid: u-1`;
        expect(extractControllerOwnerFromYaml(podYaml(reversed))).toEqual({ kind: 'StatefulSet', name: 'db', uid: 'u-2' });
    });

    it('returns null when no owner is a controller', () => {
        const owners = `  ownerReferences:
  - apiVersion: v1
    kind: ConfigMap
    name: cm
    uid: u-1`;
        expect(extractControllerOwnerFromYaml(podYaml(owners))).toBeNull();
    });

    it('accepts the compact "controller:true" spelling', () => {
        const owners = `  ownerReferences:
  - controller:true
    kind: Job
    name: batch-1
    uid: u-5`;
        expect(extractControllerOwnerFromYaml(podYaml(owners))).toEqual({ kind: 'Job', name: 'batch-1', uid: 'u-5' });
    });

    it('QUIRK: an owner entry without uid picks up the next uid line in the document', () => {
        // The last entry's text runs to the end of the metadata block, so the object's own
        // uid is reported. Pinned so that a refactor changes it knowingly, not by accident.
        const owners = `  ownerReferences:
  - controller: true
    kind: Job
    name: batch-1`;
        expect(extractControllerOwnerFromYaml(podYaml(owners))).toEqual({
            kind: 'Job', name: 'batch-1', uid: '11111111-2222-3333-4444-555555555555',
        });
    });

    it('finds a block at the end of the document only when a newline follows it', () => {
        const body = `metadata:\n  name: x\n  ownerReferences:\n  - controller: true\n    kind: DaemonSet\n    name: ds\n    uid: u-9`;
        expect(extractControllerOwnerFromYaml(`${body}\n`)).toEqual({ kind: 'DaemonSet', name: 'ds', uid: 'u-9' });
        // QUIRK: without the trailing newline the block terminator never matches.
        expect(extractControllerOwnerFromYaml(body)).toBeNull();
    });

    it('works with an indented-sequence style (items indented under the key)', () => {
        const owners = `  ownerReferences:
    - controller: true
      kind: ReplicaSet
      name: nested
      uid: u-3`;
        expect(extractControllerOwnerFromYaml(podYaml(owners))).toEqual({ kind: 'ReplicaSet', name: 'nested', uid: 'u-3' });
    });

    it('handles CRLF line endings', () => {
        const yaml = podYaml(replicaSetOwner).replace(/\n/g, '\r\n');
        expect(extractControllerOwnerFromYaml(yaml)).toMatchObject({ kind: 'ReplicaSet', name: 'web-abc' });
    });

    it('requires both kind and name on the controller entry', () => {
        const owners = `  ownerReferences:
  - controller: true
    kind: ReplicaSet`;
        expect(extractControllerOwnerFromYaml(podYaml(owners))).toBeNull();
    });
});

describe('extractControllerOwnerFromYaml on large documents', () => {
    const managedFields = (count: number) => Array.from({ length: count }, (_, i) => `  - apiVersion: v1
    fieldsType: FieldsV1
    fieldsV1:
      f:metadata:
        f:labels:
          .: {}
          f:app-${i}: {}
    manager: manager-${i}
    operation: Update
    time: "2024-01-01T00:00:00Z"`).join('\n');

    it('finds an owner placed after a very large managedFields block', () => {
        const yaml = `apiVersion: v1
kind: Pod
metadata:
  managedFields:
${managedFields(3000)}
  name: big-pod
${replicaSetOwner}
  resourceVersion: "1"
spec:
  containers: []
`;
        expect(yaml.length).toBeGreaterThan(300_000);
        expect(extractControllerOwnerFromYaml(yaml)).toEqual({
            kind: 'ReplicaSet', name: 'web-abc', uid: 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee',
        });
    });

    it('finds the controller among many owner entries', () => {
        const entries = Array.from({ length: 500 }, (_, i) =>
            `  - apiVersion: v1\n    kind: ConfigMap\n    name: cm-${i}\n    uid: uid-${i}`).join('\n');
        const owners = `  ownerReferences:\n${entries}\n  - controller: true\n    kind: Deployment\n    name: last\n    uid: uid-last`;
        expect(extractControllerOwnerFromYaml(podYaml(owners))).toEqual({ kind: 'Deployment', name: 'last', uid: 'uid-last' });
    });

    it('returns null quickly-and-correctly for a huge document with no owner block', () => {
        const yaml = `metadata:\n  managedFields:\n${managedFields(3000)}\n  name: orphan\n`;
        expect(extractControllerOwnerFromYaml(yaml)).toBeNull();
    });

    it('returns null for a huge document whose owner block has no controller', () => {
        const entries = Array.from({ length: 2000 }, (_, i) =>
            `  - apiVersion: v1\n    kind: ConfigMap\n    name: cm-${i}\n    uid: uid-${i}`).join('\n');
        const yaml = podYaml(`  ownerReferences:\n${entries}`, managedFields(500));
        expect(extractControllerOwnerFromYaml(yaml)).toBeNull();
    });
});
