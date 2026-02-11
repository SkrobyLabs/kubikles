import { describe, it, expect } from 'vitest';
import { extractControllerOwnerFromYaml } from '../../utils/k8s-helpers';

describe('extractControllerOwnerFromYaml', () => {
    it('returns null for null/undefined input', () => {
        expect(extractControllerOwnerFromYaml(null)).toBeNull();
        expect(extractControllerOwnerFromYaml(undefined)).toBeNull();
        expect(extractControllerOwnerFromYaml('')).toBeNull();
    });

    it('returns null when no ownerReferences present', () => {
        const yaml = `apiVersion: v1
kind: Pod
metadata:
  name: standalone-pod
  namespace: default
spec:
  containers:
    - name: app
      image: nginx`;
        expect(extractControllerOwnerFromYaml(yaml)).toBeNull();
    });

    it('extracts controller owner from ReplicaSet', () => {
        const yaml = `apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  namespace: default
  ownerReferences:
    - apiVersion: apps/v1
      kind: ReplicaSet
      name: my-deploy-abc123
      uid: 12345-abcde
      controller: true
      blockOwnerDeletion: true
spec:
  containers:
    - name: app`;
        const result = extractControllerOwnerFromYaml(yaml);
        expect(result).toEqual({
            kind: 'ReplicaSet',
            name: 'my-deploy-abc123',
            uid: '12345-abcde',
        });
    });

    it('extracts controller owner from Job (CronJob child)', () => {
        const yaml = `apiVersion: v1
kind: Pod
metadata:
  name: cron-pod
  namespace: default
  ownerReferences:
    - apiVersion: batch/v1
      kind: Job
      name: my-cron-12345
      uid: job-uid-123
      controller: true
spec:
  containers:
    - name: worker`;
        const result = extractControllerOwnerFromYaml(yaml);
        expect(result).toEqual({
            kind: 'Job',
            name: 'my-cron-12345',
            uid: 'job-uid-123',
        });
    });

    it('returns null when no owner has controller: true', () => {
        const yaml = `apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  ownerReferences:
    - apiVersion: apps/v1
      kind: ReplicaSet
      name: my-rs
      uid: abc123
      controller: false
spec:
  containers:
    - name: app`;
        expect(extractControllerOwnerFromYaml(yaml)).toBeNull();
    });

    it('picks the controller owner from multiple ownerReferences', () => {
        const yaml = `apiVersion: v1
kind: Pod
metadata:
  name: multi-owner-pod
  ownerReferences:
    - apiVersion: v1
      kind: Service
      name: my-svc
      uid: svc-uid
      controller: false
    - apiVersion: apps/v1
      kind: StatefulSet
      name: my-sts
      uid: sts-uid
      controller: true
spec:
  containers:
    - name: app`;
        const result = extractControllerOwnerFromYaml(yaml);
        expect(result).toEqual({
            kind: 'StatefulSet',
            name: 'my-sts',
            uid: 'sts-uid',
        });
    });

    it('handles controller:true without space', () => {
        const yaml = `apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  ownerReferences:
    - apiVersion: apps/v1
      kind: DaemonSet
      name: my-ds
      uid: ds-uid
      controller:true
spec:
  containers:
    - name: app`;
        const result = extractControllerOwnerFromYaml(yaml);
        expect(result).toEqual({
            kind: 'DaemonSet',
            name: 'my-ds',
            uid: 'ds-uid',
        });
    });

    it('returns null uid when uid is not present', () => {
        const yaml = `apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  ownerReferences:
    - apiVersion: apps/v1
      kind: ReplicaSet
      name: my-rs
      controller: true
spec:
  containers:
    - name: app`;
        const result = extractControllerOwnerFromYaml(yaml);
        expect(result).toEqual({
            kind: 'ReplicaSet',
            name: 'my-rs',
            uid: null,
        });
    });
});
