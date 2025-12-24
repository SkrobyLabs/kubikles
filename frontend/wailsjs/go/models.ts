export namespace helm {
	
	export class ChartVersion {
	    version: string;
	    appVersion: string;
	    description: string;
	    // Go type: time
	    created: any;
	    deprecated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChartVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.appVersion = source["appVersion"];
	        this.description = source["description"];
	        this.created = this.convertValues(source["created"], null);
	        this.deprecated = source["deprecated"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ChartSource {
	    repoName: string;
	    repoUrl: string;
	    priority: number;
	    chartName: string;
	    versions: ChartVersion[];
	    isOci: boolean;
	    ociRepository: string;
	
	    static createFrom(source: any = {}) {
	        return new ChartSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repoName = source["repoName"];
	        this.repoUrl = source["repoUrl"];
	        this.priority = source["priority"];
	        this.chartName = source["chartName"];
	        this.versions = this.convertValues(source["versions"], ChartVersion);
	        this.isOci = source["isOci"];
	        this.ociRepository = source["ociRepository"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ChartSearchResult {
	    found: boolean;
	    source?: ChartSource;
	    log: string;
	    duration: number;
	
	    static createFrom(source: any = {}) {
	        return new ChartSearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.found = source["found"];
	        this.source = this.convertValues(source["source"], ChartSource);
	        this.log = source["log"];
	        this.duration = source["duration"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ChartSourceInfo {
	    name: string;
	    url: string;
	    isOci: boolean;
	    isAcr: boolean;
	    priority: number;
	
	    static createFrom(source: any = {}) {
	        return new ChartSourceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.url = source["url"];
	        this.isOci = source["isOci"];
	        this.isAcr = source["isAcr"];
	        this.priority = source["priority"];
	    }
	}
	
	export class OCIRegistry {
	    url: string;
	    username: string;
	    authenticated: boolean;
	    isAcr: boolean;
	    priority: number;
	
	    static createFrom(source: any = {}) {
	        return new OCIRegistry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.username = source["username"];
	        this.authenticated = source["authenticated"];
	        this.isAcr = source["isAcr"];
	        this.priority = source["priority"];
	    }
	}
	export class Release {
	    name: string;
	    namespace: string;
	    revision: number;
	    status: string;
	    chart: string;
	    chartVersion: string;
	    appVersion: string;
	    // Go type: time
	    updated: any;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new Release(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.revision = source["revision"];
	        this.status = source["status"];
	        this.chart = source["chart"];
	        this.chartVersion = source["chartVersion"];
	        this.appVersion = source["appVersion"];
	        this.updated = this.convertValues(source["updated"], null);
	        this.description = source["description"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ReleaseDetail {
	    name: string;
	    namespace: string;
	    revision: number;
	    status: string;
	    chart: string;
	    chartVersion: string;
	    appVersion: string;
	    // Go type: time
	    updated: any;
	    description: string;
	    values: Record<string, any>;
	    computedValues: Record<string, any>;
	    notes: string;
	    manifest: string;
	
	    static createFrom(source: any = {}) {
	        return new ReleaseDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.revision = source["revision"];
	        this.status = source["status"];
	        this.chart = source["chart"];
	        this.chartVersion = source["chartVersion"];
	        this.appVersion = source["appVersion"];
	        this.updated = this.convertValues(source["updated"], null);
	        this.description = source["description"];
	        this.values = source["values"];
	        this.computedValues = source["computedValues"];
	        this.notes = source["notes"];
	        this.manifest = source["manifest"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ReleaseHistory {
	    revision: number;
	    status: string;
	    chart: string;
	    appVersion: string;
	    // Go type: time
	    updated: any;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new ReleaseHistory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.revision = source["revision"];
	        this.status = source["status"];
	        this.chart = source["chart"];
	        this.appVersion = source["appVersion"];
	        this.updated = this.convertValues(source["updated"], null);
	        this.description = source["description"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Repository {
	    name: string;
	    url: string;
	    priority: number;
	
	    static createFrom(source: any = {}) {
	        return new Repository(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.url = source["url"];
	        this.priority = source["priority"];
	    }
	}
	export class ResourceReference {
	    kind: string;
	    name: string;
	    namespace: string;
	
	    static createFrom(source: any = {}) {
	        return new ResourceReference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	    }
	}
	export class UpgradeOptions {
	    repoName: string;
	    repoUrl: string;
	    chartName: string;
	    version: string;
	    values: Record<string, any>;
	    reuseValues: boolean;
	    resetValues: boolean;
	    force: boolean;
	    wait: boolean;
	    timeout: number;
	    isOci: boolean;
	    ociRepository: string;
	
	    static createFrom(source: any = {}) {
	        return new UpgradeOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repoName = source["repoName"];
	        this.repoUrl = source["repoUrl"];
	        this.chartName = source["chartName"];
	        this.version = source["version"];
	        this.values = source["values"];
	        this.reuseValues = source["reuseValues"];
	        this.resetValues = source["resetValues"];
	        this.force = source["force"];
	        this.wait = source["wait"];
	        this.timeout = source["timeout"];
	        this.isOci = source["isOci"];
	        this.ociRepository = source["ociRepository"];
	    }
	}

}

export namespace intstr {
	
	export class IntOrString {
	    Type: number;
	    IntVal: number;
	    StrVal: string;
	
	    static createFrom(source: any = {}) {
	        return new IntOrString(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Type = source["Type"];
	        this.IntVal = source["IntVal"];
	        this.StrVal = source["StrVal"];
	    }
	}

}

export namespace k8s {
	
	export class DependencyEdge {
	    source: string;
	    target: string;
	    relation: string;
	
	    static createFrom(source: any = {}) {
	        return new DependencyEdge(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.target = source["target"];
	        this.relation = source["relation"];
	    }
	}
	export class DependencyNode {
	    id: string;
	    kind: string;
	    name: string;
	    namespace?: string;
	    status?: string;
	
	    static createFrom(source: any = {}) {
	        return new DependencyNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.status = source["status"];
	    }
	}
	export class DependencyGraph {
	    nodes: DependencyNode[];
	    edges: DependencyEdge[];
	
	    static createFrom(source: any = {}) {
	        return new DependencyGraph(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nodes = this.convertValues(source["nodes"], DependencyNode);
	        this.edges = this.convertValues(source["edges"], DependencyEdge);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class NodeMetrics {
	    name: string;
	    cpuUsage: number;
	    memoryUsage: number;
	    cpuCapacity: number;
	    memCapacity: number;
	    cpuRequested: number;
	    memRequested: number;
	    cpuCommitted: number;
	    memCommitted: number;
	
	    static createFrom(source: any = {}) {
	        return new NodeMetrics(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.cpuUsage = source["cpuUsage"];
	        this.memoryUsage = source["memoryUsage"];
	        this.cpuCapacity = source["cpuCapacity"];
	        this.memCapacity = source["memCapacity"];
	        this.cpuRequested = source["cpuRequested"];
	        this.memRequested = source["memRequested"];
	        this.cpuCommitted = source["cpuCommitted"];
	        this.memCommitted = source["memCommitted"];
	    }
	}
	export class NodeMetricsResult {
	    available: boolean;
	    metrics: NodeMetrics[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new NodeMetricsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.metrics = this.convertValues(source["metrics"], NodeMetrics);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PodMetrics {
	    namespace: string;
	    name: string;
	    nodeName: string;
	    cpuUsage: number;
	    memoryUsage: number;
	    cpuRequested: number;
	    memRequested: number;
	    cpuCommitted: number;
	    memCommitted: number;
	    nodeCpuCapacity: number;
	    nodeMemCapacity: number;
	
	    static createFrom(source: any = {}) {
	        return new PodMetrics(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.nodeName = source["nodeName"];
	        this.cpuUsage = source["cpuUsage"];
	        this.memoryUsage = source["memoryUsage"];
	        this.cpuRequested = source["cpuRequested"];
	        this.memRequested = source["memRequested"];
	        this.cpuCommitted = source["cpuCommitted"];
	        this.memCommitted = source["memCommitted"];
	        this.nodeCpuCapacity = source["nodeCpuCapacity"];
	        this.nodeMemCapacity = source["nodeMemCapacity"];
	    }
	}
	export class PodMetricsResult {
	    available: boolean;
	    metrics: PodMetrics[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new PodMetricsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.metrics = this.convertValues(source["metrics"], PodMetrics);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PrinterColumn {
	    name: string;
	    type: string;
	    jsonPath: string;
	    description: string;
	    priority: number;
	
	    static createFrom(source: any = {}) {
	        return new PrinterColumn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.jsonPath = source["jsonPath"];
	        this.description = source["description"];
	        this.priority = source["priority"];
	    }
	}

}

export namespace main {
	
	export class PortForwardConfig {
	    id: string;
	    context: string;
	    namespace: string;
	    resourceType: string;
	    resourceName: string;
	    localPort: number;
	    remotePort: number;
	    label: string;
	    favorite: boolean;
	    wasRunning: boolean;
	    https: boolean;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new PortForwardConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.context = source["context"];
	        this.namespace = source["namespace"];
	        this.resourceType = source["resourceType"];
	        this.resourceName = source["resourceName"];
	        this.localPort = source["localPort"];
	        this.remotePort = source["remotePort"];
	        this.label = source["label"];
	        this.favorite = source["favorite"];
	        this.wasRunning = source["wasRunning"];
	        this.https = source["https"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ActivePortForward {
	    config: PortForwardConfig;
	    status: string;
	    error: string;
	    // Go type: time
	    startedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new ActivePortForward(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], PortForwardConfig);
	        this.status = source["status"];
	        this.error = source["error"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IngressController {
	    namespace: string;
	    name: string;
	    type: string;
	    httpPort: number;
	    httpsPort: number;
	
	    static createFrom(source: any = {}) {
	        return new IngressController(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.httpPort = source["httpPort"];
	        this.httpsPort = source["httpsPort"];
	    }
	}
	export class IngressForwardState {
	    active: boolean;
	    status: string;
	    error?: string;
	    controller?: IngressController;
	    localHttpPort: number;
	    localHttpsPort: number;
	    hostnames: string[];
	    portForwardIds: string[];
	    hostsFileUpdated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new IngressForwardState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = source["active"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.controller = this.convertValues(source["controller"], IngressController);
	        this.localHttpPort = source["localHttpPort"];
	        this.localHttpsPort = source["localHttpsPort"];
	        this.hostnames = source["hostnames"];
	        this.portForwardIds = source["portForwardIds"];
	        this.hostsFileUpdated = source["hostsFileUpdated"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LogChunkResult {
	    logs: string;
	    hasMore: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LogChunkResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.logs = source["logs"];
	        this.hasMore = source["hasMore"];
	    }
	}
	export class NodeDebugPodResult {
	    podName: string;
	    namespace: string;
	
	    static createFrom(source: any = {}) {
	        return new NodeDebugPodResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.podName = source["podName"];
	        this.namespace = source["namespace"];
	    }
	}
	export class PodLogEntry {
	    podName: string;
	    containerName: string;
	    logs: string;
	
	    static createFrom(source: any = {}) {
	        return new PodLogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.podName = source["podName"];
	        this.containerName = source["containerName"];
	        this.logs = source["logs"];
	    }
	}

}

export namespace resource {
	
	export class Quantity {
	    Format: string;
	
	    static createFrom(source: any = {}) {
	        return new Quantity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Format = source["Format"];
	    }
	}

}

export namespace v1 {
	
	export class PodAntiAffinity {
	    requiredDuringSchedulingIgnoredDuringExecution?: PodAffinityTerm[];
	    preferredDuringSchedulingIgnoredDuringExecution?: WeightedPodAffinityTerm[];
	
	    static createFrom(source: any = {}) {
	        return new PodAntiAffinity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requiredDuringSchedulingIgnoredDuringExecution = this.convertValues(source["requiredDuringSchedulingIgnoredDuringExecution"], PodAffinityTerm);
	        this.preferredDuringSchedulingIgnoredDuringExecution = this.convertValues(source["preferredDuringSchedulingIgnoredDuringExecution"], WeightedPodAffinityTerm);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WeightedPodAffinityTerm {
	    weight: number;
	    podAffinityTerm: PodAffinityTerm;
	
	    static createFrom(source: any = {}) {
	        return new WeightedPodAffinityTerm(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.weight = source["weight"];
	        this.podAffinityTerm = this.convertValues(source["podAffinityTerm"], PodAffinityTerm);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LabelSelectorRequirement {
	    key: string;
	    operator: string;
	    values?: string[];
	
	    static createFrom(source: any = {}) {
	        return new LabelSelectorRequirement(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.operator = source["operator"];
	        this.values = source["values"];
	    }
	}
	export class LabelSelector {
	    matchLabels?: Record<string, string>;
	    matchExpressions?: LabelSelectorRequirement[];
	
	    static createFrom(source: any = {}) {
	        return new LabelSelector(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.matchLabels = source["matchLabels"];
	        this.matchExpressions = this.convertValues(source["matchExpressions"], LabelSelectorRequirement);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PodAffinityTerm {
	    labelSelector?: LabelSelector;
	    namespaces?: string[];
	    topologyKey: string;
	    namespaceSelector?: LabelSelector;
	    matchLabelKeys?: string[];
	    mismatchLabelKeys?: string[];
	
	    static createFrom(source: any = {}) {
	        return new PodAffinityTerm(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.labelSelector = this.convertValues(source["labelSelector"], LabelSelector);
	        this.namespaces = source["namespaces"];
	        this.topologyKey = source["topologyKey"];
	        this.namespaceSelector = this.convertValues(source["namespaceSelector"], LabelSelector);
	        this.matchLabelKeys = source["matchLabelKeys"];
	        this.mismatchLabelKeys = source["mismatchLabelKeys"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PodAffinity {
	    requiredDuringSchedulingIgnoredDuringExecution?: PodAffinityTerm[];
	    preferredDuringSchedulingIgnoredDuringExecution?: WeightedPodAffinityTerm[];
	
	    static createFrom(source: any = {}) {
	        return new PodAffinity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requiredDuringSchedulingIgnoredDuringExecution = this.convertValues(source["requiredDuringSchedulingIgnoredDuringExecution"], PodAffinityTerm);
	        this.preferredDuringSchedulingIgnoredDuringExecution = this.convertValues(source["preferredDuringSchedulingIgnoredDuringExecution"], WeightedPodAffinityTerm);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PreferredSchedulingTerm {
	    weight: number;
	    preference: NodeSelectorTerm;
	
	    static createFrom(source: any = {}) {
	        return new PreferredSchedulingTerm(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.weight = source["weight"];
	        this.preference = this.convertValues(source["preference"], NodeSelectorTerm);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NodeSelectorRequirement {
	    key: string;
	    operator: string;
	    values?: string[];
	
	    static createFrom(source: any = {}) {
	        return new NodeSelectorRequirement(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.operator = source["operator"];
	        this.values = source["values"];
	    }
	}
	export class NodeSelectorTerm {
	    matchExpressions?: NodeSelectorRequirement[];
	    matchFields?: NodeSelectorRequirement[];
	
	    static createFrom(source: any = {}) {
	        return new NodeSelectorTerm(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.matchExpressions = this.convertValues(source["matchExpressions"], NodeSelectorRequirement);
	        this.matchFields = this.convertValues(source["matchFields"], NodeSelectorRequirement);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NodeSelector {
	    nodeSelectorTerms: NodeSelectorTerm[];
	
	    static createFrom(source: any = {}) {
	        return new NodeSelector(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nodeSelectorTerms = this.convertValues(source["nodeSelectorTerms"], NodeSelectorTerm);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NodeAffinity {
	    requiredDuringSchedulingIgnoredDuringExecution?: NodeSelector;
	    preferredDuringSchedulingIgnoredDuringExecution?: PreferredSchedulingTerm[];
	
	    static createFrom(source: any = {}) {
	        return new NodeAffinity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requiredDuringSchedulingIgnoredDuringExecution = this.convertValues(source["requiredDuringSchedulingIgnoredDuringExecution"], NodeSelector);
	        this.preferredDuringSchedulingIgnoredDuringExecution = this.convertValues(source["preferredDuringSchedulingIgnoredDuringExecution"], PreferredSchedulingTerm);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Affinity {
	    nodeAffinity?: NodeAffinity;
	    podAffinity?: PodAffinity;
	    podAntiAffinity?: PodAntiAffinity;
	
	    static createFrom(source: any = {}) {
	        return new Affinity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nodeAffinity = this.convertValues(source["nodeAffinity"], NodeAffinity);
	        this.podAffinity = this.convertValues(source["podAffinity"], PodAffinity);
	        this.podAntiAffinity = this.convertValues(source["podAntiAffinity"], PodAntiAffinity);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AggregationRule {
	    clusterRoleSelectors?: LabelSelector[];
	
	    static createFrom(source: any = {}) {
	        return new AggregationRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.clusterRoleSelectors = this.convertValues(source["clusterRoleSelectors"], LabelSelector);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AppArmorProfile {
	    type: string;
	    localhostProfile?: string;
	
	    static createFrom(source: any = {}) {
	        return new AppArmorProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.localhostProfile = source["localhostProfile"];
	    }
	}
	export class AttachedVolume {
	    name: string;
	    devicePath: string;
	
	    static createFrom(source: any = {}) {
	        return new AttachedVolume(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.devicePath = source["devicePath"];
	    }
	}
	export class Capabilities {
	    add?: string[];
	    drop?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Capabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.add = source["add"];
	        this.drop = source["drop"];
	    }
	}
	export class ClientIPConfig {
	    timeoutSeconds?: number;
	
	    static createFrom(source: any = {}) {
	        return new ClientIPConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timeoutSeconds = source["timeoutSeconds"];
	    }
	}
	export class PolicyRule {
	    verbs: string[];
	    apiGroups?: string[];
	    resources?: string[];
	    resourceNames?: string[];
	    nonResourceURLs?: string[];
	
	    static createFrom(source: any = {}) {
	        return new PolicyRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.verbs = source["verbs"];
	        this.apiGroups = source["apiGroups"];
	        this.resources = source["resources"];
	        this.resourceNames = source["resourceNames"];
	        this.nonResourceURLs = source["nonResourceURLs"];
	    }
	}
	export class FieldsV1 {
	
	
	    static createFrom(source: any = {}) {
	        return new FieldsV1(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class ManagedFieldsEntry {
	    manager?: string;
	    operation?: string;
	    apiVersion?: string;
	    time?: Time;
	    fieldsType?: string;
	    // Go type: FieldsV1
	    fieldsV1?: any;
	    subresource?: string;
	
	    static createFrom(source: any = {}) {
	        return new ManagedFieldsEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.manager = source["manager"];
	        this.operation = source["operation"];
	        this.apiVersion = source["apiVersion"];
	        this.time = this.convertValues(source["time"], Time);
	        this.fieldsType = source["fieldsType"];
	        this.fieldsV1 = this.convertValues(source["fieldsV1"], null);
	        this.subresource = source["subresource"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class OwnerReference {
	    apiVersion: string;
	    kind: string;
	    name: string;
	    uid: string;
	    controller?: boolean;
	    blockOwnerDeletion?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OwnerReference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiVersion = source["apiVersion"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.uid = source["uid"];
	        this.controller = source["controller"];
	        this.blockOwnerDeletion = source["blockOwnerDeletion"];
	    }
	}
	export class Time {
	
	
	    static createFrom(source: any = {}) {
	        return new Time(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class ClusterRole {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    rules: PolicyRule[];
	    aggregationRule?: AggregationRule;
	
	    static createFrom(source: any = {}) {
	        return new ClusterRole(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.rules = this.convertValues(source["rules"], PolicyRule);
	        this.aggregationRule = this.convertValues(source["aggregationRule"], AggregationRule);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RoleRef {
	    apiGroup: string;
	    kind: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new RoleRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiGroup = source["apiGroup"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	    }
	}
	export class Subject {
	    kind: string;
	    apiGroup?: string;
	    name: string;
	    namespace?: string;
	
	    static createFrom(source: any = {}) {
	        return new Subject(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiGroup = source["apiGroup"];
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	    }
	}
	export class ClusterRoleBinding {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    subjects?: Subject[];
	    roleRef: RoleRef;
	
	    static createFrom(source: any = {}) {
	        return new ClusterRoleBinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.subjects = this.convertValues(source["subjects"], Subject);
	        this.roleRef = this.convertValues(source["roleRef"], RoleRef);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Condition {
	    type: string;
	    status: string;
	    observedGeneration?: number;
	    lastTransitionTime: Time;
	    reason: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Condition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.status = source["status"];
	        this.observedGeneration = source["observedGeneration"];
	        this.lastTransitionTime = this.convertValues(source["lastTransitionTime"], Time);
	        this.reason = source["reason"];
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConfigMap {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    immutable?: boolean;
	    data?: Record<string, string>;
	    binaryData?: Record<string, Array<number>>;
	
	    static createFrom(source: any = {}) {
	        return new ConfigMap(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.immutable = source["immutable"];
	        this.data = source["data"];
	        this.binaryData = source["binaryData"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConfigMapEnvSource {
	    name?: string;
	    optional?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ConfigMapEnvSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.optional = source["optional"];
	    }
	}
	export class ConfigMapKeySelector {
	    name?: string;
	    key: string;
	    optional?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ConfigMapKeySelector(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.key = source["key"];
	        this.optional = source["optional"];
	    }
	}
	export class ConfigMapNodeConfigSource {
	    namespace: string;
	    name: string;
	    uid?: string;
	    resourceVersion?: string;
	    kubeletConfigKey: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigMapNodeConfigSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.kubeletConfigKey = source["kubeletConfigKey"];
	    }
	}
	export class SeccompProfile {
	    type: string;
	    localhostProfile?: string;
	
	    static createFrom(source: any = {}) {
	        return new SeccompProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.localhostProfile = source["localhostProfile"];
	    }
	}
	export class WindowsSecurityContextOptions {
	    gmsaCredentialSpecName?: string;
	    gmsaCredentialSpec?: string;
	    runAsUserName?: string;
	    hostProcess?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WindowsSecurityContextOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gmsaCredentialSpecName = source["gmsaCredentialSpecName"];
	        this.gmsaCredentialSpec = source["gmsaCredentialSpec"];
	        this.runAsUserName = source["runAsUserName"];
	        this.hostProcess = source["hostProcess"];
	    }
	}
	export class SELinuxOptions {
	    user?: string;
	    role?: string;
	    type?: string;
	    level?: string;
	
	    static createFrom(source: any = {}) {
	        return new SELinuxOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user = source["user"];
	        this.role = source["role"];
	        this.type = source["type"];
	        this.level = source["level"];
	    }
	}
	export class SecurityContext {
	    capabilities?: Capabilities;
	    privileged?: boolean;
	    seLinuxOptions?: SELinuxOptions;
	    windowsOptions?: WindowsSecurityContextOptions;
	    runAsUser?: number;
	    runAsGroup?: number;
	    runAsNonRoot?: boolean;
	    readOnlyRootFilesystem?: boolean;
	    allowPrivilegeEscalation?: boolean;
	    procMount?: string;
	    seccompProfile?: SeccompProfile;
	    appArmorProfile?: AppArmorProfile;
	
	    static createFrom(source: any = {}) {
	        return new SecurityContext(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.capabilities = this.convertValues(source["capabilities"], Capabilities);
	        this.privileged = source["privileged"];
	        this.seLinuxOptions = this.convertValues(source["seLinuxOptions"], SELinuxOptions);
	        this.windowsOptions = this.convertValues(source["windowsOptions"], WindowsSecurityContextOptions);
	        this.runAsUser = source["runAsUser"];
	        this.runAsGroup = source["runAsGroup"];
	        this.runAsNonRoot = source["runAsNonRoot"];
	        this.readOnlyRootFilesystem = source["readOnlyRootFilesystem"];
	        this.allowPrivilegeEscalation = source["allowPrivilegeEscalation"];
	        this.procMount = source["procMount"];
	        this.seccompProfile = this.convertValues(source["seccompProfile"], SeccompProfile);
	        this.appArmorProfile = this.convertValues(source["appArmorProfile"], AppArmorProfile);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SleepAction {
	    seconds: number;
	
	    static createFrom(source: any = {}) {
	        return new SleepAction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seconds = source["seconds"];
	    }
	}
	export class LifecycleHandler {
	    exec?: ExecAction;
	    httpGet?: HTTPGetAction;
	    tcpSocket?: TCPSocketAction;
	    sleep?: SleepAction;
	
	    static createFrom(source: any = {}) {
	        return new LifecycleHandler(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exec = this.convertValues(source["exec"], ExecAction);
	        this.httpGet = this.convertValues(source["httpGet"], HTTPGetAction);
	        this.tcpSocket = this.convertValues(source["tcpSocket"], TCPSocketAction);
	        this.sleep = this.convertValues(source["sleep"], SleepAction);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Lifecycle {
	    postStart?: LifecycleHandler;
	    preStop?: LifecycleHandler;
	    stopSignal?: string;
	
	    static createFrom(source: any = {}) {
	        return new Lifecycle(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.postStart = this.convertValues(source["postStart"], LifecycleHandler);
	        this.preStop = this.convertValues(source["preStop"], LifecycleHandler);
	        this.stopSignal = source["stopSignal"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GRPCAction {
	    port: number;
	    service?: string;
	
	    static createFrom(source: any = {}) {
	        return new GRPCAction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.service = source["service"];
	    }
	}
	export class TCPSocketAction {
	    port: intstr.IntOrString;
	    host?: string;
	
	    static createFrom(source: any = {}) {
	        return new TCPSocketAction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = this.convertValues(source["port"], intstr.IntOrString);
	        this.host = source["host"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HTTPHeader {
	    name: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new HTTPHeader(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}
	export class HTTPGetAction {
	    path?: string;
	    port: intstr.IntOrString;
	    host?: string;
	    scheme?: string;
	    httpHeaders?: HTTPHeader[];
	
	    static createFrom(source: any = {}) {
	        return new HTTPGetAction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.port = this.convertValues(source["port"], intstr.IntOrString);
	        this.host = source["host"];
	        this.scheme = source["scheme"];
	        this.httpHeaders = this.convertValues(source["httpHeaders"], HTTPHeader);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ExecAction {
	    command?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ExecAction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.command = source["command"];
	    }
	}
	export class Probe {
	    exec?: ExecAction;
	    httpGet?: HTTPGetAction;
	    tcpSocket?: TCPSocketAction;
	    // Go type: GRPCAction
	    grpc?: any;
	    initialDelaySeconds?: number;
	    timeoutSeconds?: number;
	    periodSeconds?: number;
	    successThreshold?: number;
	    failureThreshold?: number;
	    terminationGracePeriodSeconds?: number;
	
	    static createFrom(source: any = {}) {
	        return new Probe(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exec = this.convertValues(source["exec"], ExecAction);
	        this.httpGet = this.convertValues(source["httpGet"], HTTPGetAction);
	        this.tcpSocket = this.convertValues(source["tcpSocket"], TCPSocketAction);
	        this.grpc = this.convertValues(source["grpc"], null);
	        this.initialDelaySeconds = source["initialDelaySeconds"];
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.periodSeconds = source["periodSeconds"];
	        this.successThreshold = source["successThreshold"];
	        this.failureThreshold = source["failureThreshold"];
	        this.terminationGracePeriodSeconds = source["terminationGracePeriodSeconds"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class VolumeDevice {
	    name: string;
	    devicePath: string;
	
	    static createFrom(source: any = {}) {
	        return new VolumeDevice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.devicePath = source["devicePath"];
	    }
	}
	export class VolumeMount {
	    name: string;
	    readOnly?: boolean;
	    recursiveReadOnly?: string;
	    mountPath: string;
	    subPath?: string;
	    mountPropagation?: string;
	    subPathExpr?: string;
	
	    static createFrom(source: any = {}) {
	        return new VolumeMount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.readOnly = source["readOnly"];
	        this.recursiveReadOnly = source["recursiveReadOnly"];
	        this.mountPath = source["mountPath"];
	        this.subPath = source["subPath"];
	        this.mountPropagation = source["mountPropagation"];
	        this.subPathExpr = source["subPathExpr"];
	    }
	}
	export class ContainerRestartRuleOnExitCodes {
	    operator?: string;
	    values?: number[];
	
	    static createFrom(source: any = {}) {
	        return new ContainerRestartRuleOnExitCodes(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.operator = source["operator"];
	        this.values = source["values"];
	    }
	}
	export class ContainerRestartRule {
	    action?: string;
	    exitCodes?: ContainerRestartRuleOnExitCodes;
	
	    static createFrom(source: any = {}) {
	        return new ContainerRestartRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action = source["action"];
	        this.exitCodes = this.convertValues(source["exitCodes"], ContainerRestartRuleOnExitCodes);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ContainerResizePolicy {
	    resourceName: string;
	    restartPolicy: string;
	
	    static createFrom(source: any = {}) {
	        return new ContainerResizePolicy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resourceName = source["resourceName"];
	        this.restartPolicy = source["restartPolicy"];
	    }
	}
	export class ResourceClaim {
	    name: string;
	    request?: string;
	
	    static createFrom(source: any = {}) {
	        return new ResourceClaim(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.request = source["request"];
	    }
	}
	export class ResourceRequirements {
	    limits?: Record<string, resource.Quantity>;
	    requests?: Record<string, resource.Quantity>;
	    claims?: ResourceClaim[];
	
	    static createFrom(source: any = {}) {
	        return new ResourceRequirements(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.limits = this.convertValues(source["limits"], resource.Quantity, true);
	        this.requests = this.convertValues(source["requests"], resource.Quantity, true);
	        this.claims = this.convertValues(source["claims"], ResourceClaim);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FileKeySelector {
	    volumeName: string;
	    path: string;
	    key: string;
	    optional?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileKeySelector(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.volumeName = source["volumeName"];
	        this.path = source["path"];
	        this.key = source["key"];
	        this.optional = source["optional"];
	    }
	}
	export class SecretKeySelector {
	    name?: string;
	    key: string;
	    optional?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SecretKeySelector(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.key = source["key"];
	        this.optional = source["optional"];
	    }
	}
	export class ResourceFieldSelector {
	    containerName?: string;
	    resource: string;
	    divisor?: resource.Quantity;
	
	    static createFrom(source: any = {}) {
	        return new ResourceFieldSelector(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.containerName = source["containerName"];
	        this.resource = source["resource"];
	        this.divisor = this.convertValues(source["divisor"], resource.Quantity);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ObjectFieldSelector {
	    apiVersion?: string;
	    fieldPath: string;
	
	    static createFrom(source: any = {}) {
	        return new ObjectFieldSelector(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiVersion = source["apiVersion"];
	        this.fieldPath = source["fieldPath"];
	    }
	}
	export class EnvVarSource {
	    fieldRef?: ObjectFieldSelector;
	    resourceFieldRef?: ResourceFieldSelector;
	    configMapKeyRef?: ConfigMapKeySelector;
	    secretKeyRef?: SecretKeySelector;
	    fileKeyRef?: FileKeySelector;
	
	    static createFrom(source: any = {}) {
	        return new EnvVarSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fieldRef = this.convertValues(source["fieldRef"], ObjectFieldSelector);
	        this.resourceFieldRef = this.convertValues(source["resourceFieldRef"], ResourceFieldSelector);
	        this.configMapKeyRef = this.convertValues(source["configMapKeyRef"], ConfigMapKeySelector);
	        this.secretKeyRef = this.convertValues(source["secretKeyRef"], SecretKeySelector);
	        this.fileKeyRef = this.convertValues(source["fileKeyRef"], FileKeySelector);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class EnvVar {
	    name: string;
	    value?: string;
	    valueFrom?: EnvVarSource;
	
	    static createFrom(source: any = {}) {
	        return new EnvVar(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	        this.valueFrom = this.convertValues(source["valueFrom"], EnvVarSource);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SecretEnvSource {
	    name?: string;
	    optional?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SecretEnvSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.optional = source["optional"];
	    }
	}
	export class EnvFromSource {
	    prefix?: string;
	    configMapRef?: ConfigMapEnvSource;
	    secretRef?: SecretEnvSource;
	
	    static createFrom(source: any = {}) {
	        return new EnvFromSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.prefix = source["prefix"];
	        this.configMapRef = this.convertValues(source["configMapRef"], ConfigMapEnvSource);
	        this.secretRef = this.convertValues(source["secretRef"], SecretEnvSource);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ContainerPort {
	    name?: string;
	    hostPort?: number;
	    containerPort: number;
	    protocol?: string;
	    hostIP?: string;
	
	    static createFrom(source: any = {}) {
	        return new ContainerPort(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.hostPort = source["hostPort"];
	        this.containerPort = source["containerPort"];
	        this.protocol = source["protocol"];
	        this.hostIP = source["hostIP"];
	    }
	}
	export class Container {
	    name: string;
	    image?: string;
	    command?: string[];
	    args?: string[];
	    workingDir?: string;
	    ports?: ContainerPort[];
	    envFrom?: EnvFromSource[];
	    env?: EnvVar[];
	    resources?: ResourceRequirements;
	    resizePolicy?: ContainerResizePolicy[];
	    restartPolicy?: string;
	    restartPolicyRules?: ContainerRestartRule[];
	    volumeMounts?: VolumeMount[];
	    volumeDevices?: VolumeDevice[];
	    livenessProbe?: Probe;
	    readinessProbe?: Probe;
	    startupProbe?: Probe;
	    lifecycle?: Lifecycle;
	    terminationMessagePath?: string;
	    terminationMessagePolicy?: string;
	    imagePullPolicy?: string;
	    securityContext?: SecurityContext;
	    stdin?: boolean;
	    stdinOnce?: boolean;
	    tty?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Container(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.image = source["image"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.workingDir = source["workingDir"];
	        this.ports = this.convertValues(source["ports"], ContainerPort);
	        this.envFrom = this.convertValues(source["envFrom"], EnvFromSource);
	        this.env = this.convertValues(source["env"], EnvVar);
	        this.resources = this.convertValues(source["resources"], ResourceRequirements);
	        this.resizePolicy = this.convertValues(source["resizePolicy"], ContainerResizePolicy);
	        this.restartPolicy = source["restartPolicy"];
	        this.restartPolicyRules = this.convertValues(source["restartPolicyRules"], ContainerRestartRule);
	        this.volumeMounts = this.convertValues(source["volumeMounts"], VolumeMount);
	        this.volumeDevices = this.convertValues(source["volumeDevices"], VolumeDevice);
	        this.livenessProbe = this.convertValues(source["livenessProbe"], Probe);
	        this.readinessProbe = this.convertValues(source["readinessProbe"], Probe);
	        this.startupProbe = this.convertValues(source["startupProbe"], Probe);
	        this.lifecycle = this.convertValues(source["lifecycle"], Lifecycle);
	        this.terminationMessagePath = source["terminationMessagePath"];
	        this.terminationMessagePolicy = source["terminationMessagePolicy"];
	        this.imagePullPolicy = source["imagePullPolicy"];
	        this.securityContext = this.convertValues(source["securityContext"], SecurityContext);
	        this.stdin = source["stdin"];
	        this.stdinOnce = source["stdinOnce"];
	        this.tty = source["tty"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ContainerExtendedResourceRequest {
	    containerName: string;
	    resourceName: string;
	    requestName: string;
	
	    static createFrom(source: any = {}) {
	        return new ContainerExtendedResourceRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.containerName = source["containerName"];
	        this.resourceName = source["resourceName"];
	        this.requestName = source["requestName"];
	    }
	}
	export class ContainerImage {
	    names: string[];
	    sizeBytes?: number;
	
	    static createFrom(source: any = {}) {
	        return new ContainerImage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.names = source["names"];
	        this.sizeBytes = source["sizeBytes"];
	    }
	}
	
	
	
	
	export class ContainerStateTerminated {
	    exitCode: number;
	    signal?: number;
	    reason?: string;
	    message?: string;
	    startedAt?: Time;
	    finishedAt?: Time;
	    containerID?: string;
	
	    static createFrom(source: any = {}) {
	        return new ContainerStateTerminated(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exitCode = source["exitCode"];
	        this.signal = source["signal"];
	        this.reason = source["reason"];
	        this.message = source["message"];
	        this.startedAt = this.convertValues(source["startedAt"], Time);
	        this.finishedAt = this.convertValues(source["finishedAt"], Time);
	        this.containerID = source["containerID"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ContainerStateRunning {
	    startedAt?: Time;
	
	    static createFrom(source: any = {}) {
	        return new ContainerStateRunning(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.startedAt = this.convertValues(source["startedAt"], Time);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ContainerStateWaiting {
	    reason?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new ContainerStateWaiting(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reason = source["reason"];
	        this.message = source["message"];
	    }
	}
	export class ContainerState {
	    waiting?: ContainerStateWaiting;
	    running?: ContainerStateRunning;
	    terminated?: ContainerStateTerminated;
	
	    static createFrom(source: any = {}) {
	        return new ContainerState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.waiting = this.convertValues(source["waiting"], ContainerStateWaiting);
	        this.running = this.convertValues(source["running"], ContainerStateRunning);
	        this.terminated = this.convertValues(source["terminated"], ContainerStateTerminated);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class ResourceHealth {
	    resourceID: string;
	    health?: string;
	
	    static createFrom(source: any = {}) {
	        return new ResourceHealth(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resourceID = source["resourceID"];
	        this.health = source["health"];
	    }
	}
	export class ResourceStatus {
	    name: string;
	    resources?: ResourceHealth[];
	
	    static createFrom(source: any = {}) {
	        return new ResourceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.resources = this.convertValues(source["resources"], ResourceHealth);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LinuxContainerUser {
	    uid: number;
	    gid: number;
	    supplementalGroups?: number[];
	
	    static createFrom(source: any = {}) {
	        return new LinuxContainerUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uid = source["uid"];
	        this.gid = source["gid"];
	        this.supplementalGroups = source["supplementalGroups"];
	    }
	}
	export class ContainerUser {
	    linux?: LinuxContainerUser;
	
	    static createFrom(source: any = {}) {
	        return new ContainerUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.linux = this.convertValues(source["linux"], LinuxContainerUser);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class VolumeMountStatus {
	    name: string;
	    mountPath: string;
	    readOnly?: boolean;
	    recursiveReadOnly?: string;
	
	    static createFrom(source: any = {}) {
	        return new VolumeMountStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.mountPath = source["mountPath"];
	        this.readOnly = source["readOnly"];
	        this.recursiveReadOnly = source["recursiveReadOnly"];
	    }
	}
	export class ContainerStatus {
	    name: string;
	    state?: ContainerState;
	    lastState?: ContainerState;
	    ready: boolean;
	    restartCount: number;
	    image: string;
	    imageID: string;
	    containerID?: string;
	    started?: boolean;
	    allocatedResources?: Record<string, resource.Quantity>;
	    resources?: ResourceRequirements;
	    volumeMounts?: VolumeMountStatus[];
	    user?: ContainerUser;
	    allocatedResourcesStatus?: ResourceStatus[];
	    stopSignal?: string;
	
	    static createFrom(source: any = {}) {
	        return new ContainerStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.state = this.convertValues(source["state"], ContainerState);
	        this.lastState = this.convertValues(source["lastState"], ContainerState);
	        this.ready = source["ready"];
	        this.restartCount = source["restartCount"];
	        this.image = source["image"];
	        this.imageID = source["imageID"];
	        this.containerID = source["containerID"];
	        this.started = source["started"];
	        this.allocatedResources = this.convertValues(source["allocatedResources"], resource.Quantity, true);
	        this.resources = this.convertValues(source["resources"], ResourceRequirements);
	        this.volumeMounts = this.convertValues(source["volumeMounts"], VolumeMountStatus);
	        this.user = this.convertValues(source["user"], ContainerUser);
	        this.allocatedResourcesStatus = this.convertValues(source["allocatedResourcesStatus"], ResourceStatus);
	        this.stopSignal = source["stopSignal"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ObjectReference {
	    kind?: string;
	    namespace?: string;
	    name?: string;
	    uid?: string;
	    apiVersion?: string;
	    resourceVersion?: string;
	    fieldPath?: string;
	
	    static createFrom(source: any = {}) {
	        return new ObjectReference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.uid = source["uid"];
	        this.apiVersion = source["apiVersion"];
	        this.resourceVersion = source["resourceVersion"];
	        this.fieldPath = source["fieldPath"];
	    }
	}
	export class CronJobStatus {
	    active?: ObjectReference[];
	    lastScheduleTime?: Time;
	    lastSuccessfulTime?: Time;
	
	    static createFrom(source: any = {}) {
	        return new CronJobStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = this.convertValues(source["active"], ObjectReference);
	        this.lastScheduleTime = this.convertValues(source["lastScheduleTime"], Time);
	        this.lastSuccessfulTime = this.convertValues(source["lastSuccessfulTime"], Time);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PodResourceClaim {
	    name: string;
	    resourceClaimName?: string;
	    resourceClaimTemplateName?: string;
	
	    static createFrom(source: any = {}) {
	        return new PodResourceClaim(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.resourceClaimName = source["resourceClaimName"];
	        this.resourceClaimTemplateName = source["resourceClaimTemplateName"];
	    }
	}
	export class PodSchedulingGate {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new PodSchedulingGate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}
	export class PodOS {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new PodOS(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}
	export class TopologySpreadConstraint {
	    maxSkew: number;
	    topologyKey: string;
	    whenUnsatisfiable: string;
	    labelSelector?: LabelSelector;
	    minDomains?: number;
	    nodeAffinityPolicy?: string;
	    nodeTaintsPolicy?: string;
	    matchLabelKeys?: string[];
	
	    static createFrom(source: any = {}) {
	        return new TopologySpreadConstraint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.maxSkew = source["maxSkew"];
	        this.topologyKey = source["topologyKey"];
	        this.whenUnsatisfiable = source["whenUnsatisfiable"];
	        this.labelSelector = this.convertValues(source["labelSelector"], LabelSelector);
	        this.minDomains = source["minDomains"];
	        this.nodeAffinityPolicy = source["nodeAffinityPolicy"];
	        this.nodeTaintsPolicy = source["nodeTaintsPolicy"];
	        this.matchLabelKeys = source["matchLabelKeys"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PodReadinessGate {
	    conditionType: string;
	
	    static createFrom(source: any = {}) {
	        return new PodReadinessGate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conditionType = source["conditionType"];
	    }
	}
	export class PodDNSConfigOption {
	    name?: string;
	    value?: string;
	
	    static createFrom(source: any = {}) {
	        return new PodDNSConfigOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}
	export class PodDNSConfig {
	    nameservers?: string[];
	    searches?: string[];
	    options?: PodDNSConfigOption[];
	
	    static createFrom(source: any = {}) {
	        return new PodDNSConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nameservers = source["nameservers"];
	        this.searches = source["searches"];
	        this.options = this.convertValues(source["options"], PodDNSConfigOption);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HostAlias {
	    ip: string;
	    hostnames?: string[];
	
	    static createFrom(source: any = {}) {
	        return new HostAlias(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.hostnames = source["hostnames"];
	    }
	}
	export class Toleration {
	    key?: string;
	    operator?: string;
	    value?: string;
	    effect?: string;
	    tolerationSeconds?: number;
	
	    static createFrom(source: any = {}) {
	        return new Toleration(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.operator = source["operator"];
	        this.value = source["value"];
	        this.effect = source["effect"];
	        this.tolerationSeconds = source["tolerationSeconds"];
	    }
	}
	export class Sysctl {
	    name: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new Sysctl(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}
	export class PodSecurityContext {
	    seLinuxOptions?: SELinuxOptions;
	    windowsOptions?: WindowsSecurityContextOptions;
	    runAsUser?: number;
	    runAsGroup?: number;
	    runAsNonRoot?: boolean;
	    supplementalGroups?: number[];
	    supplementalGroupsPolicy?: string;
	    fsGroup?: number;
	    sysctls?: Sysctl[];
	    fsGroupChangePolicy?: string;
	    seccompProfile?: SeccompProfile;
	    appArmorProfile?: AppArmorProfile;
	    seLinuxChangePolicy?: string;
	
	    static createFrom(source: any = {}) {
	        return new PodSecurityContext(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seLinuxOptions = this.convertValues(source["seLinuxOptions"], SELinuxOptions);
	        this.windowsOptions = this.convertValues(source["windowsOptions"], WindowsSecurityContextOptions);
	        this.runAsUser = source["runAsUser"];
	        this.runAsGroup = source["runAsGroup"];
	        this.runAsNonRoot = source["runAsNonRoot"];
	        this.supplementalGroups = source["supplementalGroups"];
	        this.supplementalGroupsPolicy = source["supplementalGroupsPolicy"];
	        this.fsGroup = source["fsGroup"];
	        this.sysctls = this.convertValues(source["sysctls"], Sysctl);
	        this.fsGroupChangePolicy = source["fsGroupChangePolicy"];
	        this.seccompProfile = this.convertValues(source["seccompProfile"], SeccompProfile);
	        this.appArmorProfile = this.convertValues(source["appArmorProfile"], AppArmorProfile);
	        this.seLinuxChangePolicy = source["seLinuxChangePolicy"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class EphemeralContainer {
	    name: string;
	    image?: string;
	    command?: string[];
	    args?: string[];
	    workingDir?: string;
	    ports?: ContainerPort[];
	    envFrom?: EnvFromSource[];
	    env?: EnvVar[];
	    resources?: ResourceRequirements;
	    resizePolicy?: ContainerResizePolicy[];
	    restartPolicy?: string;
	    restartPolicyRules?: ContainerRestartRule[];
	    volumeMounts?: VolumeMount[];
	    volumeDevices?: VolumeDevice[];
	    livenessProbe?: Probe;
	    readinessProbe?: Probe;
	    startupProbe?: Probe;
	    lifecycle?: Lifecycle;
	    terminationMessagePath?: string;
	    terminationMessagePolicy?: string;
	    imagePullPolicy?: string;
	    securityContext?: SecurityContext;
	    stdin?: boolean;
	    stdinOnce?: boolean;
	    tty?: boolean;
	    targetContainerName?: string;
	
	    static createFrom(source: any = {}) {
	        return new EphemeralContainer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.image = source["image"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.workingDir = source["workingDir"];
	        this.ports = this.convertValues(source["ports"], ContainerPort);
	        this.envFrom = this.convertValues(source["envFrom"], EnvFromSource);
	        this.env = this.convertValues(source["env"], EnvVar);
	        this.resources = this.convertValues(source["resources"], ResourceRequirements);
	        this.resizePolicy = this.convertValues(source["resizePolicy"], ContainerResizePolicy);
	        this.restartPolicy = source["restartPolicy"];
	        this.restartPolicyRules = this.convertValues(source["restartPolicyRules"], ContainerRestartRule);
	        this.volumeMounts = this.convertValues(source["volumeMounts"], VolumeMount);
	        this.volumeDevices = this.convertValues(source["volumeDevices"], VolumeDevice);
	        this.livenessProbe = this.convertValues(source["livenessProbe"], Probe);
	        this.readinessProbe = this.convertValues(source["readinessProbe"], Probe);
	        this.startupProbe = this.convertValues(source["startupProbe"], Probe);
	        this.lifecycle = this.convertValues(source["lifecycle"], Lifecycle);
	        this.terminationMessagePath = source["terminationMessagePath"];
	        this.terminationMessagePolicy = source["terminationMessagePolicy"];
	        this.imagePullPolicy = source["imagePullPolicy"];
	        this.securityContext = this.convertValues(source["securityContext"], SecurityContext);
	        this.stdin = source["stdin"];
	        this.stdinOnce = source["stdinOnce"];
	        this.tty = source["tty"];
	        this.targetContainerName = source["targetContainerName"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImageVolumeSource {
	    reference?: string;
	    pullPolicy?: string;
	
	    static createFrom(source: any = {}) {
	        return new ImageVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reference = source["reference"];
	        this.pullPolicy = source["pullPolicy"];
	    }
	}
	export class TypedObjectReference {
	    apiGroup?: string;
	    kind: string;
	    name: string;
	    namespace?: string;
	
	    static createFrom(source: any = {}) {
	        return new TypedObjectReference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiGroup = source["apiGroup"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	    }
	}
	export class TypedLocalObjectReference {
	    apiGroup?: string;
	    kind: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new TypedLocalObjectReference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiGroup = source["apiGroup"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	    }
	}
	export class VolumeResourceRequirements {
	    limits?: Record<string, resource.Quantity>;
	    requests?: Record<string, resource.Quantity>;
	
	    static createFrom(source: any = {}) {
	        return new VolumeResourceRequirements(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.limits = this.convertValues(source["limits"], resource.Quantity, true);
	        this.requests = this.convertValues(source["requests"], resource.Quantity, true);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PersistentVolumeClaimSpec {
	    accessModes?: string[];
	    selector?: LabelSelector;
	    resources?: VolumeResourceRequirements;
	    volumeName?: string;
	    storageClassName?: string;
	    volumeMode?: string;
	    dataSource?: TypedLocalObjectReference;
	    dataSourceRef?: TypedObjectReference;
	    volumeAttributesClassName?: string;
	
	    static createFrom(source: any = {}) {
	        return new PersistentVolumeClaimSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.accessModes = source["accessModes"];
	        this.selector = this.convertValues(source["selector"], LabelSelector);
	        this.resources = this.convertValues(source["resources"], VolumeResourceRequirements);
	        this.volumeName = source["volumeName"];
	        this.storageClassName = source["storageClassName"];
	        this.volumeMode = source["volumeMode"];
	        this.dataSource = this.convertValues(source["dataSource"], TypedLocalObjectReference);
	        this.dataSourceRef = this.convertValues(source["dataSourceRef"], TypedObjectReference);
	        this.volumeAttributesClassName = source["volumeAttributesClassName"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PersistentVolumeClaimTemplate {
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec: PersistentVolumeClaimSpec;
	
	    static createFrom(source: any = {}) {
	        return new PersistentVolumeClaimTemplate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], PersistentVolumeClaimSpec);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class EphemeralVolumeSource {
	    // Go type: PersistentVolumeClaimTemplate
	    volumeClaimTemplate?: any;
	
	    static createFrom(source: any = {}) {
	        return new EphemeralVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.volumeClaimTemplate = this.convertValues(source["volumeClaimTemplate"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CSIVolumeSource {
	    driver: string;
	    readOnly?: boolean;
	    fsType?: string;
	    volumeAttributes?: Record<string, string>;
	    nodePublishSecretRef?: LocalObjectReference;
	
	    static createFrom(source: any = {}) {
	        return new CSIVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.driver = source["driver"];
	        this.readOnly = source["readOnly"];
	        this.fsType = source["fsType"];
	        this.volumeAttributes = source["volumeAttributes"];
	        this.nodePublishSecretRef = this.convertValues(source["nodePublishSecretRef"], LocalObjectReference);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StorageOSVolumeSource {
	    volumeName?: string;
	    volumeNamespace?: string;
	    fsType?: string;
	    readOnly?: boolean;
	    secretRef?: LocalObjectReference;
	
	    static createFrom(source: any = {}) {
	        return new StorageOSVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.volumeName = source["volumeName"];
	        this.volumeNamespace = source["volumeNamespace"];
	        this.fsType = source["fsType"];
	        this.readOnly = source["readOnly"];
	        this.secretRef = this.convertValues(source["secretRef"], LocalObjectReference);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ScaleIOVolumeSource {
	    gateway: string;
	    system: string;
	    secretRef?: LocalObjectReference;
	    sslEnabled?: boolean;
	    protectionDomain?: string;
	    storagePool?: string;
	    storageMode?: string;
	    volumeName?: string;
	    fsType?: string;
	    readOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ScaleIOVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gateway = source["gateway"];
	        this.system = source["system"];
	        this.secretRef = this.convertValues(source["secretRef"], LocalObjectReference);
	        this.sslEnabled = source["sslEnabled"];
	        this.protectionDomain = source["protectionDomain"];
	        this.storagePool = source["storagePool"];
	        this.storageMode = source["storageMode"];
	        this.volumeName = source["volumeName"];
	        this.fsType = source["fsType"];
	        this.readOnly = source["readOnly"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PortworxVolumeSource {
	    volumeID: string;
	    fsType?: string;
	    readOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PortworxVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.volumeID = source["volumeID"];
	        this.fsType = source["fsType"];
	        this.readOnly = source["readOnly"];
	    }
	}
	export class PodCertificateProjection {
	    signerName?: string;
	    keyType?: string;
	    maxExpirationSeconds?: number;
	    credentialBundlePath?: string;
	    keyPath?: string;
	    certificateChainPath?: string;
	
	    static createFrom(source: any = {}) {
	        return new PodCertificateProjection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.signerName = source["signerName"];
	        this.keyType = source["keyType"];
	        this.maxExpirationSeconds = source["maxExpirationSeconds"];
	        this.credentialBundlePath = source["credentialBundlePath"];
	        this.keyPath = source["keyPath"];
	        this.certificateChainPath = source["certificateChainPath"];
	    }
	}
	export class ClusterTrustBundleProjection {
	    name?: string;
	    signerName?: string;
	    labelSelector?: LabelSelector;
	    optional?: boolean;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ClusterTrustBundleProjection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.signerName = source["signerName"];
	        this.labelSelector = this.convertValues(source["labelSelector"], LabelSelector);
	        this.optional = source["optional"];
	        this.path = source["path"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ServiceAccountTokenProjection {
	    audience?: string;
	    expirationSeconds?: number;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ServiceAccountTokenProjection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.audience = source["audience"];
	        this.expirationSeconds = source["expirationSeconds"];
	        this.path = source["path"];
	    }
	}
	export class ConfigMapProjection {
	    name?: string;
	    items?: KeyToPath[];
	    optional?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ConfigMapProjection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.items = this.convertValues(source["items"], KeyToPath);
	        this.optional = source["optional"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DownwardAPIProjection {
	    items?: DownwardAPIVolumeFile[];
	
	    static createFrom(source: any = {}) {
	        return new DownwardAPIProjection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], DownwardAPIVolumeFile);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SecretProjection {
	    name?: string;
	    items?: KeyToPath[];
	    optional?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SecretProjection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.items = this.convertValues(source["items"], KeyToPath);
	        this.optional = source["optional"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class VolumeProjection {
	    // Go type: SecretProjection
	    secret?: any;
	    // Go type: DownwardAPIProjection
	    downwardAPI?: any;
	    // Go type: ConfigMapProjection
	    configMap?: any;
	    // Go type: ServiceAccountTokenProjection
	    serviceAccountToken?: any;
	    // Go type: ClusterTrustBundleProjection
	    clusterTrustBundle?: any;
	    // Go type: PodCertificateProjection
	    podCertificate?: any;
	
	    static createFrom(source: any = {}) {
	        return new VolumeProjection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.secret = this.convertValues(source["secret"], null);
	        this.downwardAPI = this.convertValues(source["downwardAPI"], null);
	        this.configMap = this.convertValues(source["configMap"], null);
	        this.serviceAccountToken = this.convertValues(source["serviceAccountToken"], null);
	        this.clusterTrustBundle = this.convertValues(source["clusterTrustBundle"], null);
	        this.podCertificate = this.convertValues(source["podCertificate"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProjectedVolumeSource {
	    sources: VolumeProjection[];
	    defaultMode?: number;
	
	    static createFrom(source: any = {}) {
	        return new ProjectedVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sources = this.convertValues(source["sources"], VolumeProjection);
	        this.defaultMode = source["defaultMode"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PhotonPersistentDiskVolumeSource {
	    pdID: string;
	    fsType?: string;
	
	    static createFrom(source: any = {}) {
	        return new PhotonPersistentDiskVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pdID = source["pdID"];
	        this.fsType = source["fsType"];
	    }
	}
	export class AzureDiskVolumeSource {
	    diskName: string;
	    diskURI: string;
	    cachingMode?: string;
	    fsType?: string;
	    readOnly?: boolean;
	    kind?: string;
	
	    static createFrom(source: any = {}) {
	        return new AzureDiskVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.diskName = source["diskName"];
	        this.diskURI = source["diskURI"];
	        this.cachingMode = source["cachingMode"];
	        this.fsType = source["fsType"];
	        this.readOnly = source["readOnly"];
	        this.kind = source["kind"];
	    }
	}
	export class QuobyteVolumeSource {
	    registry: string;
	    volume: string;
	    readOnly?: boolean;
	    user?: string;
	    group?: string;
	    tenant?: string;
	
	    static createFrom(source: any = {}) {
	        return new QuobyteVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.registry = source["registry"];
	        this.volume = source["volume"];
	        this.readOnly = source["readOnly"];
	        this.user = source["user"];
	        this.group = source["group"];
	        this.tenant = source["tenant"];
	    }
	}
	export class VsphereVirtualDiskVolumeSource {
	    volumePath: string;
	    fsType?: string;
	    storagePolicyName?: string;
	    storagePolicyID?: string;
	
	    static createFrom(source: any = {}) {
	        return new VsphereVirtualDiskVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.volumePath = source["volumePath"];
	        this.fsType = source["fsType"];
	        this.storagePolicyName = source["storagePolicyName"];
	        this.storagePolicyID = source["storagePolicyID"];
	    }
	}
	export class ConfigMapVolumeSource {
	    name?: string;
	    items?: KeyToPath[];
	    defaultMode?: number;
	    optional?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ConfigMapVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.items = this.convertValues(source["items"], KeyToPath);
	        this.defaultMode = source["defaultMode"];
	        this.optional = source["optional"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AzureFileVolumeSource {
	    secretName: string;
	    shareName: string;
	    readOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AzureFileVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.secretName = source["secretName"];
	        this.shareName = source["shareName"];
	        this.readOnly = source["readOnly"];
	    }
	}
	export class FCVolumeSource {
	    targetWWNs?: string[];
	    lun?: number;
	    fsType?: string;
	    readOnly?: boolean;
	    wwids?: string[];
	
	    static createFrom(source: any = {}) {
	        return new FCVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.targetWWNs = source["targetWWNs"];
	        this.lun = source["lun"];
	        this.fsType = source["fsType"];
	        this.readOnly = source["readOnly"];
	        this.wwids = source["wwids"];
	    }
	}
	export class DownwardAPIVolumeFile {
	    path: string;
	    fieldRef?: ObjectFieldSelector;
	    resourceFieldRef?: ResourceFieldSelector;
	    mode?: number;
	
	    static createFrom(source: any = {}) {
	        return new DownwardAPIVolumeFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.fieldRef = this.convertValues(source["fieldRef"], ObjectFieldSelector);
	        this.resourceFieldRef = this.convertValues(source["resourceFieldRef"], ResourceFieldSelector);
	        this.mode = source["mode"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DownwardAPIVolumeSource {
	    items?: DownwardAPIVolumeFile[];
	    defaultMode?: number;
	
	    static createFrom(source: any = {}) {
	        return new DownwardAPIVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], DownwardAPIVolumeFile);
	        this.defaultMode = source["defaultMode"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FlockerVolumeSource {
	    datasetName?: string;
	    datasetUUID?: string;
	
	    static createFrom(source: any = {}) {
	        return new FlockerVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.datasetName = source["datasetName"];
	        this.datasetUUID = source["datasetUUID"];
	    }
	}
	export class CephFSVolumeSource {
	    monitors: string[];
	    path?: string;
	    user?: string;
	    secretFile?: string;
	    secretRef?: LocalObjectReference;
	    readOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CephFSVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.monitors = source["monitors"];
	        this.path = source["path"];
	        this.user = source["user"];
	        this.secretFile = source["secretFile"];
	        this.secretRef = this.convertValues(source["secretRef"], LocalObjectReference);
	        this.readOnly = source["readOnly"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CinderVolumeSource {
	    volumeID: string;
	    fsType?: string;
	    readOnly?: boolean;
	    secretRef?: LocalObjectReference;
	
	    static createFrom(source: any = {}) {
	        return new CinderVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.volumeID = source["volumeID"];
	        this.fsType = source["fsType"];
	        this.readOnly = source["readOnly"];
	        this.secretRef = this.convertValues(source["secretRef"], LocalObjectReference);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FlexVolumeSource {
	    driver: string;
	    fsType?: string;
	    secretRef?: LocalObjectReference;
	    readOnly?: boolean;
	    options?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new FlexVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.driver = source["driver"];
	        this.fsType = source["fsType"];
	        this.secretRef = this.convertValues(source["secretRef"], LocalObjectReference);
	        this.readOnly = source["readOnly"];
	        this.options = source["options"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RBDVolumeSource {
	    monitors: string[];
	    image: string;
	    fsType?: string;
	    pool?: string;
	    user?: string;
	    keyring?: string;
	    secretRef?: LocalObjectReference;
	    readOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RBDVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.monitors = source["monitors"];
	        this.image = source["image"];
	        this.fsType = source["fsType"];
	        this.pool = source["pool"];
	        this.user = source["user"];
	        this.keyring = source["keyring"];
	        this.secretRef = this.convertValues(source["secretRef"], LocalObjectReference);
	        this.readOnly = source["readOnly"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PersistentVolumeClaimVolumeSource {
	    claimName: string;
	    readOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PersistentVolumeClaimVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.claimName = source["claimName"];
	        this.readOnly = source["readOnly"];
	    }
	}
	export class GlusterfsVolumeSource {
	    endpoints: string;
	    path: string;
	    readOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GlusterfsVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpoints = source["endpoints"];
	        this.path = source["path"];
	        this.readOnly = source["readOnly"];
	    }
	}
	export class LocalObjectReference {
	    name?: string;
	
	    static createFrom(source: any = {}) {
	        return new LocalObjectReference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}
	export class ISCSIVolumeSource {
	    targetPortal: string;
	    iqn: string;
	    lun: number;
	    iscsiInterface?: string;
	    fsType?: string;
	    readOnly?: boolean;
	    portals?: string[];
	    chapAuthDiscovery?: boolean;
	    chapAuthSession?: boolean;
	    secretRef?: LocalObjectReference;
	    initiatorName?: string;
	
	    static createFrom(source: any = {}) {
	        return new ISCSIVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.targetPortal = source["targetPortal"];
	        this.iqn = source["iqn"];
	        this.lun = source["lun"];
	        this.iscsiInterface = source["iscsiInterface"];
	        this.fsType = source["fsType"];
	        this.readOnly = source["readOnly"];
	        this.portals = source["portals"];
	        this.chapAuthDiscovery = source["chapAuthDiscovery"];
	        this.chapAuthSession = source["chapAuthSession"];
	        this.secretRef = this.convertValues(source["secretRef"], LocalObjectReference);
	        this.initiatorName = source["initiatorName"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NFSVolumeSource {
	    server: string;
	    path: string;
	    readOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NFSVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server = source["server"];
	        this.path = source["path"];
	        this.readOnly = source["readOnly"];
	    }
	}
	export class KeyToPath {
	    key: string;
	    path: string;
	    mode?: number;
	
	    static createFrom(source: any = {}) {
	        return new KeyToPath(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.path = source["path"];
	        this.mode = source["mode"];
	    }
	}
	export class SecretVolumeSource {
	    secretName?: string;
	    items?: KeyToPath[];
	    defaultMode?: number;
	    optional?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SecretVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.secretName = source["secretName"];
	        this.items = this.convertValues(source["items"], KeyToPath);
	        this.defaultMode = source["defaultMode"];
	        this.optional = source["optional"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GitRepoVolumeSource {
	    repository: string;
	    revision?: string;
	    directory?: string;
	
	    static createFrom(source: any = {}) {
	        return new GitRepoVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repository = source["repository"];
	        this.revision = source["revision"];
	        this.directory = source["directory"];
	    }
	}
	export class AWSElasticBlockStoreVolumeSource {
	    volumeID: string;
	    fsType?: string;
	    partition?: number;
	    readOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AWSElasticBlockStoreVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.volumeID = source["volumeID"];
	        this.fsType = source["fsType"];
	        this.partition = source["partition"];
	        this.readOnly = source["readOnly"];
	    }
	}
	export class GCEPersistentDiskVolumeSource {
	    pdName: string;
	    fsType?: string;
	    partition?: number;
	    readOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GCEPersistentDiskVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pdName = source["pdName"];
	        this.fsType = source["fsType"];
	        this.partition = source["partition"];
	        this.readOnly = source["readOnly"];
	    }
	}
	export class EmptyDirVolumeSource {
	    medium?: string;
	    sizeLimit?: resource.Quantity;
	
	    static createFrom(source: any = {}) {
	        return new EmptyDirVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.medium = source["medium"];
	        this.sizeLimit = this.convertValues(source["sizeLimit"], resource.Quantity);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HostPathVolumeSource {
	    path: string;
	    type?: string;
	
	    static createFrom(source: any = {}) {
	        return new HostPathVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.type = source["type"];
	    }
	}
	export class Volume {
	    name: string;
	    // Go type: HostPathVolumeSource
	    hostPath?: any;
	    // Go type: EmptyDirVolumeSource
	    emptyDir?: any;
	    // Go type: GCEPersistentDiskVolumeSource
	    gcePersistentDisk?: any;
	    // Go type: AWSElasticBlockStoreVolumeSource
	    awsElasticBlockStore?: any;
	    // Go type: GitRepoVolumeSource
	    gitRepo?: any;
	    // Go type: SecretVolumeSource
	    secret?: any;
	    // Go type: NFSVolumeSource
	    nfs?: any;
	    // Go type: ISCSIVolumeSource
	    iscsi?: any;
	    // Go type: GlusterfsVolumeSource
	    glusterfs?: any;
	    // Go type: PersistentVolumeClaimVolumeSource
	    persistentVolumeClaim?: any;
	    // Go type: RBDVolumeSource
	    rbd?: any;
	    // Go type: FlexVolumeSource
	    flexVolume?: any;
	    // Go type: CinderVolumeSource
	    cinder?: any;
	    // Go type: CephFSVolumeSource
	    cephfs?: any;
	    // Go type: FlockerVolumeSource
	    flocker?: any;
	    // Go type: DownwardAPIVolumeSource
	    downwardAPI?: any;
	    // Go type: FCVolumeSource
	    fc?: any;
	    // Go type: AzureFileVolumeSource
	    azureFile?: any;
	    // Go type: ConfigMapVolumeSource
	    configMap?: any;
	    // Go type: VsphereVirtualDiskVolumeSource
	    vsphereVolume?: any;
	    // Go type: QuobyteVolumeSource
	    quobyte?: any;
	    // Go type: AzureDiskVolumeSource
	    azureDisk?: any;
	    // Go type: PhotonPersistentDiskVolumeSource
	    photonPersistentDisk?: any;
	    // Go type: ProjectedVolumeSource
	    projected?: any;
	    // Go type: PortworxVolumeSource
	    portworxVolume?: any;
	    // Go type: ScaleIOVolumeSource
	    scaleIO?: any;
	    // Go type: StorageOSVolumeSource
	    storageos?: any;
	    // Go type: CSIVolumeSource
	    csi?: any;
	    // Go type: EphemeralVolumeSource
	    ephemeral?: any;
	    // Go type: ImageVolumeSource
	    image?: any;
	
	    static createFrom(source: any = {}) {
	        return new Volume(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.hostPath = this.convertValues(source["hostPath"], null);
	        this.emptyDir = this.convertValues(source["emptyDir"], null);
	        this.gcePersistentDisk = this.convertValues(source["gcePersistentDisk"], null);
	        this.awsElasticBlockStore = this.convertValues(source["awsElasticBlockStore"], null);
	        this.gitRepo = this.convertValues(source["gitRepo"], null);
	        this.secret = this.convertValues(source["secret"], null);
	        this.nfs = this.convertValues(source["nfs"], null);
	        this.iscsi = this.convertValues(source["iscsi"], null);
	        this.glusterfs = this.convertValues(source["glusterfs"], null);
	        this.persistentVolumeClaim = this.convertValues(source["persistentVolumeClaim"], null);
	        this.rbd = this.convertValues(source["rbd"], null);
	        this.flexVolume = this.convertValues(source["flexVolume"], null);
	        this.cinder = this.convertValues(source["cinder"], null);
	        this.cephfs = this.convertValues(source["cephfs"], null);
	        this.flocker = this.convertValues(source["flocker"], null);
	        this.downwardAPI = this.convertValues(source["downwardAPI"], null);
	        this.fc = this.convertValues(source["fc"], null);
	        this.azureFile = this.convertValues(source["azureFile"], null);
	        this.configMap = this.convertValues(source["configMap"], null);
	        this.vsphereVolume = this.convertValues(source["vsphereVolume"], null);
	        this.quobyte = this.convertValues(source["quobyte"], null);
	        this.azureDisk = this.convertValues(source["azureDisk"], null);
	        this.photonPersistentDisk = this.convertValues(source["photonPersistentDisk"], null);
	        this.projected = this.convertValues(source["projected"], null);
	        this.portworxVolume = this.convertValues(source["portworxVolume"], null);
	        this.scaleIO = this.convertValues(source["scaleIO"], null);
	        this.storageos = this.convertValues(source["storageos"], null);
	        this.csi = this.convertValues(source["csi"], null);
	        this.ephemeral = this.convertValues(source["ephemeral"], null);
	        this.image = this.convertValues(source["image"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PodSpec {
	    volumes?: Volume[];
	    initContainers?: Container[];
	    containers: Container[];
	    ephemeralContainers?: EphemeralContainer[];
	    restartPolicy?: string;
	    terminationGracePeriodSeconds?: number;
	    activeDeadlineSeconds?: number;
	    dnsPolicy?: string;
	    nodeSelector?: Record<string, string>;
	    serviceAccountName?: string;
	    serviceAccount?: string;
	    automountServiceAccountToken?: boolean;
	    nodeName?: string;
	    hostNetwork?: boolean;
	    hostPID?: boolean;
	    hostIPC?: boolean;
	    shareProcessNamespace?: boolean;
	    securityContext?: PodSecurityContext;
	    imagePullSecrets?: LocalObjectReference[];
	    hostname?: string;
	    subdomain?: string;
	    affinity?: Affinity;
	    schedulerName?: string;
	    tolerations?: Toleration[];
	    hostAliases?: HostAlias[];
	    priorityClassName?: string;
	    priority?: number;
	    dnsConfig?: PodDNSConfig;
	    readinessGates?: PodReadinessGate[];
	    runtimeClassName?: string;
	    enableServiceLinks?: boolean;
	    preemptionPolicy?: string;
	    overhead?: Record<string, resource.Quantity>;
	    topologySpreadConstraints?: TopologySpreadConstraint[];
	    setHostnameAsFQDN?: boolean;
	    os?: PodOS;
	    hostUsers?: boolean;
	    schedulingGates?: PodSchedulingGate[];
	    resourceClaims?: PodResourceClaim[];
	    resources?: ResourceRequirements;
	    hostnameOverride?: string;
	
	    static createFrom(source: any = {}) {
	        return new PodSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.volumes = this.convertValues(source["volumes"], Volume);
	        this.initContainers = this.convertValues(source["initContainers"], Container);
	        this.containers = this.convertValues(source["containers"], Container);
	        this.ephemeralContainers = this.convertValues(source["ephemeralContainers"], EphemeralContainer);
	        this.restartPolicy = source["restartPolicy"];
	        this.terminationGracePeriodSeconds = source["terminationGracePeriodSeconds"];
	        this.activeDeadlineSeconds = source["activeDeadlineSeconds"];
	        this.dnsPolicy = source["dnsPolicy"];
	        this.nodeSelector = source["nodeSelector"];
	        this.serviceAccountName = source["serviceAccountName"];
	        this.serviceAccount = source["serviceAccount"];
	        this.automountServiceAccountToken = source["automountServiceAccountToken"];
	        this.nodeName = source["nodeName"];
	        this.hostNetwork = source["hostNetwork"];
	        this.hostPID = source["hostPID"];
	        this.hostIPC = source["hostIPC"];
	        this.shareProcessNamespace = source["shareProcessNamespace"];
	        this.securityContext = this.convertValues(source["securityContext"], PodSecurityContext);
	        this.imagePullSecrets = this.convertValues(source["imagePullSecrets"], LocalObjectReference);
	        this.hostname = source["hostname"];
	        this.subdomain = source["subdomain"];
	        this.affinity = this.convertValues(source["affinity"], Affinity);
	        this.schedulerName = source["schedulerName"];
	        this.tolerations = this.convertValues(source["tolerations"], Toleration);
	        this.hostAliases = this.convertValues(source["hostAliases"], HostAlias);
	        this.priorityClassName = source["priorityClassName"];
	        this.priority = source["priority"];
	        this.dnsConfig = this.convertValues(source["dnsConfig"], PodDNSConfig);
	        this.readinessGates = this.convertValues(source["readinessGates"], PodReadinessGate);
	        this.runtimeClassName = source["runtimeClassName"];
	        this.enableServiceLinks = source["enableServiceLinks"];
	        this.preemptionPolicy = source["preemptionPolicy"];
	        this.overhead = this.convertValues(source["overhead"], resource.Quantity, true);
	        this.topologySpreadConstraints = this.convertValues(source["topologySpreadConstraints"], TopologySpreadConstraint);
	        this.setHostnameAsFQDN = source["setHostnameAsFQDN"];
	        this.os = this.convertValues(source["os"], PodOS);
	        this.hostUsers = source["hostUsers"];
	        this.schedulingGates = this.convertValues(source["schedulingGates"], PodSchedulingGate);
	        this.resourceClaims = this.convertValues(source["resourceClaims"], PodResourceClaim);
	        this.resources = this.convertValues(source["resources"], ResourceRequirements);
	        this.hostnameOverride = source["hostnameOverride"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PodTemplateSpec {
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: PodSpec;
	
	    static createFrom(source: any = {}) {
	        return new PodTemplateSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], PodSpec);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SuccessPolicyRule {
	    succeededIndexes?: string;
	    succeededCount?: number;
	
	    static createFrom(source: any = {}) {
	        return new SuccessPolicyRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.succeededIndexes = source["succeededIndexes"];
	        this.succeededCount = source["succeededCount"];
	    }
	}
	export class SuccessPolicy {
	    rules: SuccessPolicyRule[];
	
	    static createFrom(source: any = {}) {
	        return new SuccessPolicy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rules = this.convertValues(source["rules"], SuccessPolicyRule);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PodFailurePolicyOnPodConditionsPattern {
	    type: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new PodFailurePolicyOnPodConditionsPattern(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.status = source["status"];
	    }
	}
	export class PodFailurePolicyOnExitCodesRequirement {
	    containerName?: string;
	    operator: string;
	    values: number[];
	
	    static createFrom(source: any = {}) {
	        return new PodFailurePolicyOnExitCodesRequirement(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.containerName = source["containerName"];
	        this.operator = source["operator"];
	        this.values = source["values"];
	    }
	}
	export class PodFailurePolicyRule {
	    action: string;
	    onExitCodes?: PodFailurePolicyOnExitCodesRequirement;
	    onPodConditions?: PodFailurePolicyOnPodConditionsPattern[];
	
	    static createFrom(source: any = {}) {
	        return new PodFailurePolicyRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action = source["action"];
	        this.onExitCodes = this.convertValues(source["onExitCodes"], PodFailurePolicyOnExitCodesRequirement);
	        this.onPodConditions = this.convertValues(source["onPodConditions"], PodFailurePolicyOnPodConditionsPattern);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PodFailurePolicy {
	    rules: PodFailurePolicyRule[];
	
	    static createFrom(source: any = {}) {
	        return new PodFailurePolicy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rules = this.convertValues(source["rules"], PodFailurePolicyRule);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class JobSpec {
	    parallelism?: number;
	    completions?: number;
	    activeDeadlineSeconds?: number;
	    podFailurePolicy?: PodFailurePolicy;
	    successPolicy?: SuccessPolicy;
	    backoffLimit?: number;
	    backoffLimitPerIndex?: number;
	    maxFailedIndexes?: number;
	    selector?: LabelSelector;
	    manualSelector?: boolean;
	    template: PodTemplateSpec;
	    ttlSecondsAfterFinished?: number;
	    completionMode?: string;
	    suspend?: boolean;
	    podReplacementPolicy?: string;
	    managedBy?: string;
	
	    static createFrom(source: any = {}) {
	        return new JobSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.parallelism = source["parallelism"];
	        this.completions = source["completions"];
	        this.activeDeadlineSeconds = source["activeDeadlineSeconds"];
	        this.podFailurePolicy = this.convertValues(source["podFailurePolicy"], PodFailurePolicy);
	        this.successPolicy = this.convertValues(source["successPolicy"], SuccessPolicy);
	        this.backoffLimit = source["backoffLimit"];
	        this.backoffLimitPerIndex = source["backoffLimitPerIndex"];
	        this.maxFailedIndexes = source["maxFailedIndexes"];
	        this.selector = this.convertValues(source["selector"], LabelSelector);
	        this.manualSelector = source["manualSelector"];
	        this.template = this.convertValues(source["template"], PodTemplateSpec);
	        this.ttlSecondsAfterFinished = source["ttlSecondsAfterFinished"];
	        this.completionMode = source["completionMode"];
	        this.suspend = source["suspend"];
	        this.podReplacementPolicy = source["podReplacementPolicy"];
	        this.managedBy = source["managedBy"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class JobTemplateSpec {
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: JobSpec;
	
	    static createFrom(source: any = {}) {
	        return new JobTemplateSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], JobSpec);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CronJobSpec {
	    schedule: string;
	    timeZone?: string;
	    startingDeadlineSeconds?: number;
	    concurrencyPolicy?: string;
	    suspend?: boolean;
	    jobTemplate: JobTemplateSpec;
	    successfulJobsHistoryLimit?: number;
	    failedJobsHistoryLimit?: number;
	
	    static createFrom(source: any = {}) {
	        return new CronJobSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schedule = source["schedule"];
	        this.timeZone = source["timeZone"];
	        this.startingDeadlineSeconds = source["startingDeadlineSeconds"];
	        this.concurrencyPolicy = source["concurrencyPolicy"];
	        this.suspend = source["suspend"];
	        this.jobTemplate = this.convertValues(source["jobTemplate"], JobTemplateSpec);
	        this.successfulJobsHistoryLimit = source["successfulJobsHistoryLimit"];
	        this.failedJobsHistoryLimit = source["failedJobsHistoryLimit"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CronJob {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: CronJobSpec;
	    status?: CronJobStatus;
	
	    static createFrom(source: any = {}) {
	        return new CronJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], CronJobSpec);
	        this.status = this.convertValues(source["status"], CronJobStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class CustomResourceColumnDefinition {
	    name: string;
	    type: string;
	    format?: string;
	    description?: string;
	    priority?: number;
	    jsonPath: string;
	
	    static createFrom(source: any = {}) {
	        return new CustomResourceColumnDefinition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.format = source["format"];
	        this.description = source["description"];
	        this.priority = source["priority"];
	        this.jsonPath = source["jsonPath"];
	    }
	}
	export class ServiceReference {
	    namespace: string;
	    name: string;
	    path?: string;
	    port?: number;
	
	    static createFrom(source: any = {}) {
	        return new ServiceReference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.port = source["port"];
	    }
	}
	export class WebhookClientConfig {
	    url?: string;
	    service?: ServiceReference;
	    caBundle?: number[];
	
	    static createFrom(source: any = {}) {
	        return new WebhookClientConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.service = this.convertValues(source["service"], ServiceReference);
	        this.caBundle = source["caBundle"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WebhookConversion {
	    clientConfig?: WebhookClientConfig;
	    conversionReviewVersions: string[];
	
	    static createFrom(source: any = {}) {
	        return new WebhookConversion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.clientConfig = this.convertValues(source["clientConfig"], WebhookClientConfig);
	        this.conversionReviewVersions = source["conversionReviewVersions"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CustomResourceConversion {
	    strategy: string;
	    webhook?: WebhookConversion;
	
	    static createFrom(source: any = {}) {
	        return new CustomResourceConversion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.strategy = source["strategy"];
	        this.webhook = this.convertValues(source["webhook"], WebhookConversion);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CustomResourceDefinitionCondition {
	    type: string;
	    status: string;
	    lastTransitionTime?: Time;
	    reason?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new CustomResourceDefinitionCondition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.status = source["status"];
	        this.lastTransitionTime = this.convertValues(source["lastTransitionTime"], Time);
	        this.reason = source["reason"];
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CustomResourceDefinitionStatus {
	    conditions: CustomResourceDefinitionCondition[];
	    acceptedNames: CustomResourceDefinitionNames;
	    storedVersions: string[];
	
	    static createFrom(source: any = {}) {
	        return new CustomResourceDefinitionStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conditions = this.convertValues(source["conditions"], CustomResourceDefinitionCondition);
	        this.acceptedNames = this.convertValues(source["acceptedNames"], CustomResourceDefinitionNames);
	        this.storedVersions = source["storedVersions"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SelectableField {
	    jsonPath: string;
	
	    static createFrom(source: any = {}) {
	        return new SelectableField(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jsonPath = source["jsonPath"];
	    }
	}
	export class CustomResourceSubresourceScale {
	    specReplicasPath: string;
	    statusReplicasPath: string;
	    labelSelectorPath?: string;
	
	    static createFrom(source: any = {}) {
	        return new CustomResourceSubresourceScale(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.specReplicasPath = source["specReplicasPath"];
	        this.statusReplicasPath = source["statusReplicasPath"];
	        this.labelSelectorPath = source["labelSelectorPath"];
	    }
	}
	export class CustomResourceSubresourceStatus {
	
	
	    static createFrom(source: any = {}) {
	        return new CustomResourceSubresourceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class CustomResourceSubresources {
	    // Go type: CustomResourceSubresourceStatus
	    status?: any;
	    scale?: CustomResourceSubresourceScale;
	
	    static createFrom(source: any = {}) {
	        return new CustomResourceSubresources(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = this.convertValues(source["status"], null);
	        this.scale = this.convertValues(source["scale"], CustomResourceSubresourceScale);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ValidationRule {
	    rule: string;
	    message?: string;
	    messageExpression?: string;
	    reason?: string;
	    fieldPath?: string;
	    optionalOldSelf?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ValidationRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rule = source["rule"];
	        this.message = source["message"];
	        this.messageExpression = source["messageExpression"];
	        this.reason = source["reason"];
	        this.fieldPath = source["fieldPath"];
	        this.optionalOldSelf = source["optionalOldSelf"];
	    }
	}
	export class ExternalDocumentation {
	    description?: string;
	    url?: string;
	
	    static createFrom(source: any = {}) {
	        return new ExternalDocumentation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.description = source["description"];
	        this.url = source["url"];
	    }
	}
	export class JSONSchemaPropsOrStringArray {
	    Schema?: JSONSchemaProps;
	    Property: string[];
	
	    static createFrom(source: any = {}) {
	        return new JSONSchemaPropsOrStringArray(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Schema = this.convertValues(source["Schema"], JSONSchemaProps);
	        this.Property = source["Property"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class JSONSchemaPropsOrBool {
	    Allows: boolean;
	    Schema?: JSONSchemaProps;
	
	    static createFrom(source: any = {}) {
	        return new JSONSchemaPropsOrBool(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Allows = source["Allows"];
	        this.Schema = this.convertValues(source["Schema"], JSONSchemaProps);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class JSONSchemaPropsOrArray {
	    Schema?: JSONSchemaProps;
	    JSONSchemas: JSONSchemaProps[];
	
	    static createFrom(source: any = {}) {
	        return new JSONSchemaPropsOrArray(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Schema = this.convertValues(source["Schema"], JSONSchemaProps);
	        this.JSONSchemas = this.convertValues(source["JSONSchemas"], JSONSchemaProps);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class JSON {
	
	
	    static createFrom(source: any = {}) {
	        return new JSON(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class JSONSchemaProps {
	    id?: string;
	    $schema?: string;
	    $ref?: string;
	    description?: string;
	    type?: string;
	    format?: string;
	    title?: string;
	    default?: JSON;
	    maximum?: number;
	    exclusiveMaximum?: boolean;
	    minimum?: number;
	    exclusiveMinimum?: boolean;
	    maxLength?: number;
	    minLength?: number;
	    pattern?: string;
	    maxItems?: number;
	    minItems?: number;
	    uniqueItems?: boolean;
	    multipleOf?: number;
	    enum?: JSON[];
	    maxProperties?: number;
	    minProperties?: number;
	    required?: string[];
	    items?: JSONSchemaPropsOrArray;
	    allOf?: JSONSchemaProps[];
	    oneOf?: JSONSchemaProps[];
	    anyOf?: JSONSchemaProps[];
	    not?: JSONSchemaProps;
	    properties?: Record<string, JSONSchemaProps>;
	    additionalProperties?: JSONSchemaPropsOrBool;
	    patternProperties?: Record<string, JSONSchemaProps>;
	    dependencies?: Record<string, JSONSchemaPropsOrStringArray>;
	    additionalItems?: JSONSchemaPropsOrBool;
	    definitions?: Record<string, JSONSchemaProps>;
	    externalDocs?: ExternalDocumentation;
	    example?: JSON;
	    nullable?: boolean;
	    "x-kubernetes-preserve-unknown-fields"?: boolean;
	    "x-kubernetes-embedded-resource"?: boolean;
	    "x-kubernetes-int-or-string"?: boolean;
	    "x-kubernetes-list-map-keys"?: string[];
	    "x-kubernetes-list-type"?: string;
	    "x-kubernetes-map-type"?: string;
	    "x-kubernetes-validations"?: ValidationRule[];
	
	    static createFrom(source: any = {}) {
	        return new JSONSchemaProps(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.$schema = source["$schema"];
	        this.$ref = source["$ref"];
	        this.description = source["description"];
	        this.type = source["type"];
	        this.format = source["format"];
	        this.title = source["title"];
	        this.default = this.convertValues(source["default"], JSON);
	        this.maximum = source["maximum"];
	        this.exclusiveMaximum = source["exclusiveMaximum"];
	        this.minimum = source["minimum"];
	        this.exclusiveMinimum = source["exclusiveMinimum"];
	        this.maxLength = source["maxLength"];
	        this.minLength = source["minLength"];
	        this.pattern = source["pattern"];
	        this.maxItems = source["maxItems"];
	        this.minItems = source["minItems"];
	        this.uniqueItems = source["uniqueItems"];
	        this.multipleOf = source["multipleOf"];
	        this.enum = this.convertValues(source["enum"], JSON);
	        this.maxProperties = source["maxProperties"];
	        this.minProperties = source["minProperties"];
	        this.required = source["required"];
	        this.items = this.convertValues(source["items"], JSONSchemaPropsOrArray);
	        this.allOf = this.convertValues(source["allOf"], JSONSchemaProps);
	        this.oneOf = this.convertValues(source["oneOf"], JSONSchemaProps);
	        this.anyOf = this.convertValues(source["anyOf"], JSONSchemaProps);
	        this.not = this.convertValues(source["not"], JSONSchemaProps);
	        this.properties = this.convertValues(source["properties"], JSONSchemaProps, true);
	        this.additionalProperties = this.convertValues(source["additionalProperties"], JSONSchemaPropsOrBool);
	        this.patternProperties = this.convertValues(source["patternProperties"], JSONSchemaProps, true);
	        this.dependencies = this.convertValues(source["dependencies"], JSONSchemaPropsOrStringArray, true);
	        this.additionalItems = this.convertValues(source["additionalItems"], JSONSchemaPropsOrBool);
	        this.definitions = this.convertValues(source["definitions"], JSONSchemaProps, true);
	        this.externalDocs = this.convertValues(source["externalDocs"], ExternalDocumentation);
	        this.example = this.convertValues(source["example"], JSON);
	        this.nullable = source["nullable"];
	        this["x-kubernetes-preserve-unknown-fields"] = source["x-kubernetes-preserve-unknown-fields"];
	        this["x-kubernetes-embedded-resource"] = source["x-kubernetes-embedded-resource"];
	        this["x-kubernetes-int-or-string"] = source["x-kubernetes-int-or-string"];
	        this["x-kubernetes-list-map-keys"] = source["x-kubernetes-list-map-keys"];
	        this["x-kubernetes-list-type"] = source["x-kubernetes-list-type"];
	        this["x-kubernetes-map-type"] = source["x-kubernetes-map-type"];
	        this["x-kubernetes-validations"] = this.convertValues(source["x-kubernetes-validations"], ValidationRule);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CustomResourceValidation {
	    openAPIV3Schema?: JSONSchemaProps;
	
	    static createFrom(source: any = {}) {
	        return new CustomResourceValidation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.openAPIV3Schema = this.convertValues(source["openAPIV3Schema"], JSONSchemaProps);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CustomResourceDefinitionVersion {
	    name: string;
	    served: boolean;
	    storage: boolean;
	    deprecated?: boolean;
	    deprecationWarning?: string;
	    schema?: CustomResourceValidation;
	    subresources?: CustomResourceSubresources;
	    additionalPrinterColumns?: CustomResourceColumnDefinition[];
	    selectableFields?: SelectableField[];
	
	    static createFrom(source: any = {}) {
	        return new CustomResourceDefinitionVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.served = source["served"];
	        this.storage = source["storage"];
	        this.deprecated = source["deprecated"];
	        this.deprecationWarning = source["deprecationWarning"];
	        this.schema = this.convertValues(source["schema"], CustomResourceValidation);
	        this.subresources = this.convertValues(source["subresources"], CustomResourceSubresources);
	        this.additionalPrinterColumns = this.convertValues(source["additionalPrinterColumns"], CustomResourceColumnDefinition);
	        this.selectableFields = this.convertValues(source["selectableFields"], SelectableField);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CustomResourceDefinitionNames {
	    plural: string;
	    singular?: string;
	    shortNames?: string[];
	    kind: string;
	    listKind?: string;
	    categories?: string[];
	
	    static createFrom(source: any = {}) {
	        return new CustomResourceDefinitionNames(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plural = source["plural"];
	        this.singular = source["singular"];
	        this.shortNames = source["shortNames"];
	        this.kind = source["kind"];
	        this.listKind = source["listKind"];
	        this.categories = source["categories"];
	    }
	}
	export class CustomResourceDefinitionSpec {
	    group: string;
	    names: CustomResourceDefinitionNames;
	    scope: string;
	    versions: CustomResourceDefinitionVersion[];
	    conversion?: CustomResourceConversion;
	    preserveUnknownFields?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CustomResourceDefinitionSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.group = source["group"];
	        this.names = this.convertValues(source["names"], CustomResourceDefinitionNames);
	        this.scope = source["scope"];
	        this.versions = this.convertValues(source["versions"], CustomResourceDefinitionVersion);
	        this.conversion = this.convertValues(source["conversion"], CustomResourceConversion);
	        this.preserveUnknownFields = source["preserveUnknownFields"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CustomResourceDefinition {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec: CustomResourceDefinitionSpec;
	    status?: CustomResourceDefinitionStatus;
	
	    static createFrom(source: any = {}) {
	        return new CustomResourceDefinition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], CustomResourceDefinitionSpec);
	        this.status = this.convertValues(source["status"], CustomResourceDefinitionStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	
	export class DaemonEndpoint {
	    Port: number;
	
	    static createFrom(source: any = {}) {
	        return new DaemonEndpoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Port = source["Port"];
	    }
	}
	export class DaemonSetCondition {
	    type: string;
	    status: string;
	    lastTransitionTime?: Time;
	    reason?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new DaemonSetCondition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.status = source["status"];
	        this.lastTransitionTime = this.convertValues(source["lastTransitionTime"], Time);
	        this.reason = source["reason"];
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DaemonSetStatus {
	    currentNumberScheduled: number;
	    numberMisscheduled: number;
	    desiredNumberScheduled: number;
	    numberReady: number;
	    observedGeneration?: number;
	    updatedNumberScheduled?: number;
	    numberAvailable?: number;
	    numberUnavailable?: number;
	    collisionCount?: number;
	    conditions?: DaemonSetCondition[];
	
	    static createFrom(source: any = {}) {
	        return new DaemonSetStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentNumberScheduled = source["currentNumberScheduled"];
	        this.numberMisscheduled = source["numberMisscheduled"];
	        this.desiredNumberScheduled = source["desiredNumberScheduled"];
	        this.numberReady = source["numberReady"];
	        this.observedGeneration = source["observedGeneration"];
	        this.updatedNumberScheduled = source["updatedNumberScheduled"];
	        this.numberAvailable = source["numberAvailable"];
	        this.numberUnavailable = source["numberUnavailable"];
	        this.collisionCount = source["collisionCount"];
	        this.conditions = this.convertValues(source["conditions"], DaemonSetCondition);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RollingUpdateDaemonSet {
	    maxUnavailable?: intstr.IntOrString;
	    maxSurge?: intstr.IntOrString;
	
	    static createFrom(source: any = {}) {
	        return new RollingUpdateDaemonSet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.maxUnavailable = this.convertValues(source["maxUnavailable"], intstr.IntOrString);
	        this.maxSurge = this.convertValues(source["maxSurge"], intstr.IntOrString);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DaemonSetUpdateStrategy {
	    type?: string;
	    rollingUpdate?: RollingUpdateDaemonSet;
	
	    static createFrom(source: any = {}) {
	        return new DaemonSetUpdateStrategy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.rollingUpdate = this.convertValues(source["rollingUpdate"], RollingUpdateDaemonSet);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DaemonSetSpec {
	    selector?: LabelSelector;
	    template: PodTemplateSpec;
	    updateStrategy?: DaemonSetUpdateStrategy;
	    minReadySeconds?: number;
	    revisionHistoryLimit?: number;
	
	    static createFrom(source: any = {}) {
	        return new DaemonSetSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.selector = this.convertValues(source["selector"], LabelSelector);
	        this.template = this.convertValues(source["template"], PodTemplateSpec);
	        this.updateStrategy = this.convertValues(source["updateStrategy"], DaemonSetUpdateStrategy);
	        this.minReadySeconds = source["minReadySeconds"];
	        this.revisionHistoryLimit = source["revisionHistoryLimit"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DaemonSet {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: DaemonSetSpec;
	    status?: DaemonSetStatus;
	
	    static createFrom(source: any = {}) {
	        return new DaemonSet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], DaemonSetSpec);
	        this.status = this.convertValues(source["status"], DaemonSetStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	export class DeploymentCondition {
	    type: string;
	    status: string;
	    lastUpdateTime?: Time;
	    lastTransitionTime?: Time;
	    reason?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new DeploymentCondition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.status = source["status"];
	        this.lastUpdateTime = this.convertValues(source["lastUpdateTime"], Time);
	        this.lastTransitionTime = this.convertValues(source["lastTransitionTime"], Time);
	        this.reason = source["reason"];
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DeploymentStatus {
	    observedGeneration?: number;
	    replicas?: number;
	    updatedReplicas?: number;
	    readyReplicas?: number;
	    availableReplicas?: number;
	    unavailableReplicas?: number;
	    terminatingReplicas?: number;
	    conditions?: DeploymentCondition[];
	    collisionCount?: number;
	
	    static createFrom(source: any = {}) {
	        return new DeploymentStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.observedGeneration = source["observedGeneration"];
	        this.replicas = source["replicas"];
	        this.updatedReplicas = source["updatedReplicas"];
	        this.readyReplicas = source["readyReplicas"];
	        this.availableReplicas = source["availableReplicas"];
	        this.unavailableReplicas = source["unavailableReplicas"];
	        this.terminatingReplicas = source["terminatingReplicas"];
	        this.conditions = this.convertValues(source["conditions"], DeploymentCondition);
	        this.collisionCount = source["collisionCount"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RollingUpdateDeployment {
	    maxUnavailable?: intstr.IntOrString;
	    maxSurge?: intstr.IntOrString;
	
	    static createFrom(source: any = {}) {
	        return new RollingUpdateDeployment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.maxUnavailable = this.convertValues(source["maxUnavailable"], intstr.IntOrString);
	        this.maxSurge = this.convertValues(source["maxSurge"], intstr.IntOrString);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DeploymentStrategy {
	    type?: string;
	    rollingUpdate?: RollingUpdateDeployment;
	
	    static createFrom(source: any = {}) {
	        return new DeploymentStrategy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.rollingUpdate = this.convertValues(source["rollingUpdate"], RollingUpdateDeployment);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DeploymentSpec {
	    replicas?: number;
	    selector?: LabelSelector;
	    template: PodTemplateSpec;
	    strategy?: DeploymentStrategy;
	    minReadySeconds?: number;
	    revisionHistoryLimit?: number;
	    paused?: boolean;
	    progressDeadlineSeconds?: number;
	
	    static createFrom(source: any = {}) {
	        return new DeploymentSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.replicas = source["replicas"];
	        this.selector = this.convertValues(source["selector"], LabelSelector);
	        this.template = this.convertValues(source["template"], PodTemplateSpec);
	        this.strategy = this.convertValues(source["strategy"], DeploymentStrategy);
	        this.minReadySeconds = source["minReadySeconds"];
	        this.revisionHistoryLimit = source["revisionHistoryLimit"];
	        this.paused = source["paused"];
	        this.progressDeadlineSeconds = source["progressDeadlineSeconds"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Deployment {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: DeploymentSpec;
	    status?: DeploymentStatus;
	
	    static createFrom(source: any = {}) {
	        return new Deployment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], DeploymentSpec);
	        this.status = this.convertValues(source["status"], DeploymentStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	
	export class EventSeries {
	    count?: number;
	    lastObservedTime?: MicroTime;
	
	    static createFrom(source: any = {}) {
	        return new EventSeries(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.count = source["count"];
	        this.lastObservedTime = this.convertValues(source["lastObservedTime"], MicroTime);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MicroTime {
	
	
	    static createFrom(source: any = {}) {
	        return new MicroTime(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class EventSource {
	    component?: string;
	    host?: string;
	
	    static createFrom(source: any = {}) {
	        return new EventSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.component = source["component"];
	        this.host = source["host"];
	    }
	}
	export class Event {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    involvedObject: ObjectReference;
	    reason?: string;
	    message?: string;
	    source?: EventSource;
	    firstTimestamp?: Time;
	    lastTimestamp?: Time;
	    count?: number;
	    type?: string;
	    eventTime?: MicroTime;
	    series?: EventSeries;
	    action?: string;
	    related?: ObjectReference;
	    reportingComponent: string;
	    reportingInstance: string;
	
	    static createFrom(source: any = {}) {
	        return new Event(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.involvedObject = this.convertValues(source["involvedObject"], ObjectReference);
	        this.reason = source["reason"];
	        this.message = source["message"];
	        this.source = this.convertValues(source["source"], EventSource);
	        this.firstTimestamp = this.convertValues(source["firstTimestamp"], Time);
	        this.lastTimestamp = this.convertValues(source["lastTimestamp"], Time);
	        this.count = source["count"];
	        this.type = source["type"];
	        this.eventTime = this.convertValues(source["eventTime"], MicroTime);
	        this.series = this.convertValues(source["series"], EventSeries);
	        this.action = source["action"];
	        this.related = this.convertValues(source["related"], ObjectReference);
	        this.reportingComponent = source["reportingComponent"];
	        this.reportingInstance = source["reportingInstance"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	
	export class HostIP {
	    ip: string;
	
	    static createFrom(source: any = {}) {
	        return new HostIP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	    }
	}
	export class IngressPortStatus {
	    port: number;
	    protocol: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new IngressPortStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.protocol = source["protocol"];
	        this.error = source["error"];
	    }
	}
	export class IngressLoadBalancerIngress {
	    ip?: string;
	    hostname?: string;
	    ports?: IngressPortStatus[];
	
	    static createFrom(source: any = {}) {
	        return new IngressLoadBalancerIngress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.hostname = source["hostname"];
	        this.ports = this.convertValues(source["ports"], IngressPortStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IngressLoadBalancerStatus {
	    ingress?: IngressLoadBalancerIngress[];
	
	    static createFrom(source: any = {}) {
	        return new IngressLoadBalancerStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ingress = this.convertValues(source["ingress"], IngressLoadBalancerIngress);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IngressStatus {
	    loadBalancer?: IngressLoadBalancerStatus;
	
	    static createFrom(source: any = {}) {
	        return new IngressStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.loadBalancer = this.convertValues(source["loadBalancer"], IngressLoadBalancerStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HTTPIngressPath {
	    path?: string;
	    pathType?: string;
	    backend: IngressBackend;
	
	    static createFrom(source: any = {}) {
	        return new HTTPIngressPath(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.pathType = source["pathType"];
	        this.backend = this.convertValues(source["backend"], IngressBackend);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HTTPIngressRuleValue {
	    paths: HTTPIngressPath[];
	
	    static createFrom(source: any = {}) {
	        return new HTTPIngressRuleValue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.paths = this.convertValues(source["paths"], HTTPIngressPath);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IngressRule {
	    host?: string;
	    // Go type: HTTPIngressRuleValue
	    http?: any;
	
	    static createFrom(source: any = {}) {
	        return new IngressRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.http = this.convertValues(source["http"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IngressTLS {
	    hosts?: string[];
	    secretName?: string;
	
	    static createFrom(source: any = {}) {
	        return new IngressTLS(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hosts = source["hosts"];
	        this.secretName = source["secretName"];
	    }
	}
	export class ServiceBackendPort {
	    name?: string;
	    number?: number;
	
	    static createFrom(source: any = {}) {
	        return new ServiceBackendPort(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.number = source["number"];
	    }
	}
	export class IngressServiceBackend {
	    name: string;
	    port?: ServiceBackendPort;
	
	    static createFrom(source: any = {}) {
	        return new IngressServiceBackend(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.port = this.convertValues(source["port"], ServiceBackendPort);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IngressBackend {
	    service?: IngressServiceBackend;
	    resource?: TypedLocalObjectReference;
	
	    static createFrom(source: any = {}) {
	        return new IngressBackend(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.service = this.convertValues(source["service"], IngressServiceBackend);
	        this.resource = this.convertValues(source["resource"], TypedLocalObjectReference);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IngressSpec {
	    ingressClassName?: string;
	    defaultBackend?: IngressBackend;
	    tls?: IngressTLS[];
	    rules?: IngressRule[];
	
	    static createFrom(source: any = {}) {
	        return new IngressSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ingressClassName = source["ingressClassName"];
	        this.defaultBackend = this.convertValues(source["defaultBackend"], IngressBackend);
	        this.tls = this.convertValues(source["tls"], IngressTLS);
	        this.rules = this.convertValues(source["rules"], IngressRule);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Ingress {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: IngressSpec;
	    status?: IngressStatus;
	
	    static createFrom(source: any = {}) {
	        return new Ingress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], IngressSpec);
	        this.status = this.convertValues(source["status"], IngressStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class IngressClassParametersReference {
	    apiGroup?: string;
	    kind: string;
	    name: string;
	    scope?: string;
	    namespace?: string;
	
	    static createFrom(source: any = {}) {
	        return new IngressClassParametersReference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiGroup = source["apiGroup"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.scope = source["scope"];
	        this.namespace = source["namespace"];
	    }
	}
	export class IngressClassSpec {
	    controller?: string;
	    parameters?: IngressClassParametersReference;
	
	    static createFrom(source: any = {}) {
	        return new IngressClassSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.controller = source["controller"];
	        this.parameters = this.convertValues(source["parameters"], IngressClassParametersReference);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IngressClass {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: IngressClassSpec;
	
	    static createFrom(source: any = {}) {
	        return new IngressClass(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], IngressClassSpec);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	export class UncountedTerminatedPods {
	    succeeded?: string[];
	    failed?: string[];
	
	    static createFrom(source: any = {}) {
	        return new UncountedTerminatedPods(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.succeeded = source["succeeded"];
	        this.failed = source["failed"];
	    }
	}
	export class JobCondition {
	    type: string;
	    status: string;
	    lastProbeTime?: Time;
	    lastTransitionTime?: Time;
	    reason?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new JobCondition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.status = source["status"];
	        this.lastProbeTime = this.convertValues(source["lastProbeTime"], Time);
	        this.lastTransitionTime = this.convertValues(source["lastTransitionTime"], Time);
	        this.reason = source["reason"];
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class JobStatus {
	    conditions?: JobCondition[];
	    startTime?: Time;
	    completionTime?: Time;
	    active?: number;
	    succeeded?: number;
	    failed?: number;
	    terminating?: number;
	    completedIndexes?: string;
	    failedIndexes?: string;
	    uncountedTerminatedPods?: UncountedTerminatedPods;
	    ready?: number;
	
	    static createFrom(source: any = {}) {
	        return new JobStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conditions = this.convertValues(source["conditions"], JobCondition);
	        this.startTime = this.convertValues(source["startTime"], Time);
	        this.completionTime = this.convertValues(source["completionTime"], Time);
	        this.active = source["active"];
	        this.succeeded = source["succeeded"];
	        this.failed = source["failed"];
	        this.terminating = source["terminating"];
	        this.completedIndexes = source["completedIndexes"];
	        this.failedIndexes = source["failedIndexes"];
	        this.uncountedTerminatedPods = this.convertValues(source["uncountedTerminatedPods"], UncountedTerminatedPods);
	        this.ready = source["ready"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Job {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: JobSpec;
	    status?: JobStatus;
	
	    static createFrom(source: any = {}) {
	        return new Job(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], JobSpec);
	        this.status = this.convertValues(source["status"], JobStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	
	
	export class PortStatus {
	    port: number;
	    protocol: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new PortStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.protocol = source["protocol"];
	        this.error = source["error"];
	    }
	}
	export class LoadBalancerIngress {
	    ip?: string;
	    hostname?: string;
	    ipMode?: string;
	    ports?: PortStatus[];
	
	    static createFrom(source: any = {}) {
	        return new LoadBalancerIngress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.hostname = source["hostname"];
	        this.ipMode = source["ipMode"];
	        this.ports = this.convertValues(source["ports"], PortStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LoadBalancerStatus {
	    ingress?: LoadBalancerIngress[];
	
	    static createFrom(source: any = {}) {
	        return new LoadBalancerStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ingress = this.convertValues(source["ingress"], LoadBalancerIngress);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class ModifyVolumeStatus {
	    targetVolumeAttributesClassName?: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new ModifyVolumeStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.targetVolumeAttributesClassName = source["targetVolumeAttributesClassName"];
	        this.status = source["status"];
	    }
	}
	export class NamespaceCondition {
	    type: string;
	    status: string;
	    lastTransitionTime?: Time;
	    reason?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new NamespaceCondition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.status = source["status"];
	        this.lastTransitionTime = this.convertValues(source["lastTransitionTime"], Time);
	        this.reason = source["reason"];
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NamespaceStatus {
	    phase?: string;
	    conditions?: NamespaceCondition[];
	
	    static createFrom(source: any = {}) {
	        return new NamespaceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.phase = source["phase"];
	        this.conditions = this.convertValues(source["conditions"], NamespaceCondition);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NamespaceSpec {
	    finalizers?: string[];
	
	    static createFrom(source: any = {}) {
	        return new NamespaceSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.finalizers = source["finalizers"];
	    }
	}
	export class Namespace {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: NamespaceSpec;
	    status?: NamespaceStatus;
	
	    static createFrom(source: any = {}) {
	        return new Namespace(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], NamespaceSpec);
	        this.status = this.convertValues(source["status"], NamespaceStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class NodeFeatures {
	    supplementalGroupsPolicy?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NodeFeatures(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supplementalGroupsPolicy = source["supplementalGroupsPolicy"];
	    }
	}
	export class NodeRuntimeHandlerFeatures {
	    recursiveReadOnlyMounts?: boolean;
	    userNamespaces?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new NodeRuntimeHandlerFeatures(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.recursiveReadOnlyMounts = source["recursiveReadOnlyMounts"];
	        this.userNamespaces = source["userNamespaces"];
	    }
	}
	export class NodeRuntimeHandler {
	    name: string;
	    features?: NodeRuntimeHandlerFeatures;
	
	    static createFrom(source: any = {}) {
	        return new NodeRuntimeHandler(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.features = this.convertValues(source["features"], NodeRuntimeHandlerFeatures);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NodeConfigStatus {
	    assigned?: NodeConfigSource;
	    active?: NodeConfigSource;
	    lastKnownGood?: NodeConfigSource;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new NodeConfigStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.assigned = this.convertValues(source["assigned"], NodeConfigSource);
	        this.active = this.convertValues(source["active"], NodeConfigSource);
	        this.lastKnownGood = this.convertValues(source["lastKnownGood"], NodeConfigSource);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NodeSwapStatus {
	    capacity?: number;
	
	    static createFrom(source: any = {}) {
	        return new NodeSwapStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.capacity = source["capacity"];
	    }
	}
	export class NodeSystemInfo {
	    machineID: string;
	    systemUUID: string;
	    bootID: string;
	    kernelVersion: string;
	    osImage: string;
	    containerRuntimeVersion: string;
	    kubeletVersion: string;
	    kubeProxyVersion: string;
	    operatingSystem: string;
	    architecture: string;
	    swap?: NodeSwapStatus;
	
	    static createFrom(source: any = {}) {
	        return new NodeSystemInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.machineID = source["machineID"];
	        this.systemUUID = source["systemUUID"];
	        this.bootID = source["bootID"];
	        this.kernelVersion = source["kernelVersion"];
	        this.osImage = source["osImage"];
	        this.containerRuntimeVersion = source["containerRuntimeVersion"];
	        this.kubeletVersion = source["kubeletVersion"];
	        this.kubeProxyVersion = source["kubeProxyVersion"];
	        this.operatingSystem = source["operatingSystem"];
	        this.architecture = source["architecture"];
	        this.swap = this.convertValues(source["swap"], NodeSwapStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NodeDaemonEndpoints {
	    kubeletEndpoint?: DaemonEndpoint;
	
	    static createFrom(source: any = {}) {
	        return new NodeDaemonEndpoints(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kubeletEndpoint = this.convertValues(source["kubeletEndpoint"], DaemonEndpoint);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NodeAddress {
	    type: string;
	    address: string;
	
	    static createFrom(source: any = {}) {
	        return new NodeAddress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.address = source["address"];
	    }
	}
	export class NodeCondition {
	    type: string;
	    status: string;
	    lastHeartbeatTime?: Time;
	    lastTransitionTime?: Time;
	    reason?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new NodeCondition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.status = source["status"];
	        this.lastHeartbeatTime = this.convertValues(source["lastHeartbeatTime"], Time);
	        this.lastTransitionTime = this.convertValues(source["lastTransitionTime"], Time);
	        this.reason = source["reason"];
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NodeStatus {
	    capacity?: Record<string, resource.Quantity>;
	    allocatable?: Record<string, resource.Quantity>;
	    phase?: string;
	    conditions?: NodeCondition[];
	    addresses?: NodeAddress[];
	    daemonEndpoints?: NodeDaemonEndpoints;
	    nodeInfo?: NodeSystemInfo;
	    images?: ContainerImage[];
	    volumesInUse?: string[];
	    volumesAttached?: AttachedVolume[];
	    config?: NodeConfigStatus;
	    runtimeHandlers?: NodeRuntimeHandler[];
	    features?: NodeFeatures;
	
	    static createFrom(source: any = {}) {
	        return new NodeStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.capacity = this.convertValues(source["capacity"], resource.Quantity, true);
	        this.allocatable = this.convertValues(source["allocatable"], resource.Quantity, true);
	        this.phase = source["phase"];
	        this.conditions = this.convertValues(source["conditions"], NodeCondition);
	        this.addresses = this.convertValues(source["addresses"], NodeAddress);
	        this.daemonEndpoints = this.convertValues(source["daemonEndpoints"], NodeDaemonEndpoints);
	        this.nodeInfo = this.convertValues(source["nodeInfo"], NodeSystemInfo);
	        this.images = this.convertValues(source["images"], ContainerImage);
	        this.volumesInUse = source["volumesInUse"];
	        this.volumesAttached = this.convertValues(source["volumesAttached"], AttachedVolume);
	        this.config = this.convertValues(source["config"], NodeConfigStatus);
	        this.runtimeHandlers = this.convertValues(source["runtimeHandlers"], NodeRuntimeHandler);
	        this.features = this.convertValues(source["features"], NodeFeatures);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NodeConfigSource {
	    configMap?: ConfigMapNodeConfigSource;
	
	    static createFrom(source: any = {}) {
	        return new NodeConfigSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configMap = this.convertValues(source["configMap"], ConfigMapNodeConfigSource);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Taint {
	    key: string;
	    value?: string;
	    effect: string;
	    timeAdded?: Time;
	
	    static createFrom(source: any = {}) {
	        return new Taint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	        this.effect = source["effect"];
	        this.timeAdded = this.convertValues(source["timeAdded"], Time);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NodeSpec {
	    podCIDR?: string;
	    podCIDRs?: string[];
	    providerID?: string;
	    unschedulable?: boolean;
	    taints?: Taint[];
	    configSource?: NodeConfigSource;
	    externalID?: string;
	
	    static createFrom(source: any = {}) {
	        return new NodeSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.podCIDR = source["podCIDR"];
	        this.podCIDRs = source["podCIDRs"];
	        this.providerID = source["providerID"];
	        this.unschedulable = source["unschedulable"];
	        this.taints = this.convertValues(source["taints"], Taint);
	        this.configSource = this.convertValues(source["configSource"], NodeConfigSource);
	        this.externalID = source["externalID"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Node {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: NodeSpec;
	    status?: NodeStatus;
	
	    static createFrom(source: any = {}) {
	        return new Node(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], NodeSpec);
	        this.status = this.convertValues(source["status"], NodeStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	export class PersistentVolumeStatus {
	    phase?: string;
	    message?: string;
	    reason?: string;
	    lastPhaseTransitionTime?: Time;
	
	    static createFrom(source: any = {}) {
	        return new PersistentVolumeStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.phase = source["phase"];
	        this.message = source["message"];
	        this.reason = source["reason"];
	        this.lastPhaseTransitionTime = this.convertValues(source["lastPhaseTransitionTime"], Time);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class VolumeNodeAffinity {
	    required?: NodeSelector;
	
	    static createFrom(source: any = {}) {
	        return new VolumeNodeAffinity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.required = this.convertValues(source["required"], NodeSelector);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CSIPersistentVolumeSource {
	    driver: string;
	    volumeHandle: string;
	    readOnly?: boolean;
	    fsType?: string;
	    volumeAttributes?: Record<string, string>;
	    // Go type: SecretReference
	    controllerPublishSecretRef?: any;
	    // Go type: SecretReference
	    nodeStageSecretRef?: any;
	    // Go type: SecretReference
	    nodePublishSecretRef?: any;
	    // Go type: SecretReference
	    controllerExpandSecretRef?: any;
	    // Go type: SecretReference
	    nodeExpandSecretRef?: any;
	
	    static createFrom(source: any = {}) {
	        return new CSIPersistentVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.driver = source["driver"];
	        this.volumeHandle = source["volumeHandle"];
	        this.readOnly = source["readOnly"];
	        this.fsType = source["fsType"];
	        this.volumeAttributes = source["volumeAttributes"];
	        this.controllerPublishSecretRef = this.convertValues(source["controllerPublishSecretRef"], null);
	        this.nodeStageSecretRef = this.convertValues(source["nodeStageSecretRef"], null);
	        this.nodePublishSecretRef = this.convertValues(source["nodePublishSecretRef"], null);
	        this.controllerExpandSecretRef = this.convertValues(source["controllerExpandSecretRef"], null);
	        this.nodeExpandSecretRef = this.convertValues(source["nodeExpandSecretRef"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StorageOSPersistentVolumeSource {
	    volumeName?: string;
	    volumeNamespace?: string;
	    fsType?: string;
	    readOnly?: boolean;
	    secretRef?: ObjectReference;
	
	    static createFrom(source: any = {}) {
	        return new StorageOSPersistentVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.volumeName = source["volumeName"];
	        this.volumeNamespace = source["volumeNamespace"];
	        this.fsType = source["fsType"];
	        this.readOnly = source["readOnly"];
	        this.secretRef = this.convertValues(source["secretRef"], ObjectReference);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LocalVolumeSource {
	    path: string;
	    fsType?: string;
	
	    static createFrom(source: any = {}) {
	        return new LocalVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.fsType = source["fsType"];
	    }
	}
	export class ScaleIOPersistentVolumeSource {
	    gateway: string;
	    system: string;
	    // Go type: SecretReference
	    secretRef?: any;
	    sslEnabled?: boolean;
	    protectionDomain?: string;
	    storagePool?: string;
	    storageMode?: string;
	    volumeName?: string;
	    fsType?: string;
	    readOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ScaleIOPersistentVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gateway = source["gateway"];
	        this.system = source["system"];
	        this.secretRef = this.convertValues(source["secretRef"], null);
	        this.sslEnabled = source["sslEnabled"];
	        this.protectionDomain = source["protectionDomain"];
	        this.storagePool = source["storagePool"];
	        this.storageMode = source["storageMode"];
	        this.volumeName = source["volumeName"];
	        this.fsType = source["fsType"];
	        this.readOnly = source["readOnly"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AzureFilePersistentVolumeSource {
	    secretName: string;
	    shareName: string;
	    readOnly?: boolean;
	    secretNamespace?: string;
	
	    static createFrom(source: any = {}) {
	        return new AzureFilePersistentVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.secretName = source["secretName"];
	        this.shareName = source["shareName"];
	        this.readOnly = source["readOnly"];
	        this.secretNamespace = source["secretNamespace"];
	    }
	}
	export class FlexPersistentVolumeSource {
	    driver: string;
	    fsType?: string;
	    // Go type: SecretReference
	    secretRef?: any;
	    readOnly?: boolean;
	    options?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new FlexPersistentVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.driver = source["driver"];
	        this.fsType = source["fsType"];
	        this.secretRef = this.convertValues(source["secretRef"], null);
	        this.readOnly = source["readOnly"];
	        this.options = source["options"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CephFSPersistentVolumeSource {
	    monitors: string[];
	    path?: string;
	    user?: string;
	    secretFile?: string;
	    // Go type: SecretReference
	    secretRef?: any;
	    readOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CephFSPersistentVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.monitors = source["monitors"];
	        this.path = source["path"];
	        this.user = source["user"];
	        this.secretFile = source["secretFile"];
	        this.secretRef = this.convertValues(source["secretRef"], null);
	        this.readOnly = source["readOnly"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CinderPersistentVolumeSource {
	    volumeID: string;
	    fsType?: string;
	    readOnly?: boolean;
	    // Go type: SecretReference
	    secretRef?: any;
	
	    static createFrom(source: any = {}) {
	        return new CinderPersistentVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.volumeID = source["volumeID"];
	        this.fsType = source["fsType"];
	        this.readOnly = source["readOnly"];
	        this.secretRef = this.convertValues(source["secretRef"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ISCSIPersistentVolumeSource {
	    targetPortal: string;
	    iqn: string;
	    lun: number;
	    iscsiInterface?: string;
	    fsType?: string;
	    readOnly?: boolean;
	    portals?: string[];
	    chapAuthDiscovery?: boolean;
	    chapAuthSession?: boolean;
	    // Go type: SecretReference
	    secretRef?: any;
	    initiatorName?: string;
	
	    static createFrom(source: any = {}) {
	        return new ISCSIPersistentVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.targetPortal = source["targetPortal"];
	        this.iqn = source["iqn"];
	        this.lun = source["lun"];
	        this.iscsiInterface = source["iscsiInterface"];
	        this.fsType = source["fsType"];
	        this.readOnly = source["readOnly"];
	        this.portals = source["portals"];
	        this.chapAuthDiscovery = source["chapAuthDiscovery"];
	        this.chapAuthSession = source["chapAuthSession"];
	        this.secretRef = this.convertValues(source["secretRef"], null);
	        this.initiatorName = source["initiatorName"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SecretReference {
	    name?: string;
	    namespace?: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretReference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	    }
	}
	export class RBDPersistentVolumeSource {
	    monitors: string[];
	    image: string;
	    fsType?: string;
	    pool?: string;
	    user?: string;
	    keyring?: string;
	    // Go type: SecretReference
	    secretRef?: any;
	    readOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RBDPersistentVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.monitors = source["monitors"];
	        this.image = source["image"];
	        this.fsType = source["fsType"];
	        this.pool = source["pool"];
	        this.user = source["user"];
	        this.keyring = source["keyring"];
	        this.secretRef = this.convertValues(source["secretRef"], null);
	        this.readOnly = source["readOnly"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GlusterfsPersistentVolumeSource {
	    endpoints: string;
	    path: string;
	    readOnly?: boolean;
	    endpointsNamespace?: string;
	
	    static createFrom(source: any = {}) {
	        return new GlusterfsPersistentVolumeSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpoints = source["endpoints"];
	        this.path = source["path"];
	        this.readOnly = source["readOnly"];
	        this.endpointsNamespace = source["endpointsNamespace"];
	    }
	}
	export class PersistentVolumeSpec {
	    capacity?: Record<string, resource.Quantity>;
	    // Go type: GCEPersistentDiskVolumeSource
	    gcePersistentDisk?: any;
	    // Go type: AWSElasticBlockStoreVolumeSource
	    awsElasticBlockStore?: any;
	    // Go type: HostPathVolumeSource
	    hostPath?: any;
	    // Go type: GlusterfsPersistentVolumeSource
	    glusterfs?: any;
	    // Go type: NFSVolumeSource
	    nfs?: any;
	    // Go type: RBDPersistentVolumeSource
	    rbd?: any;
	    // Go type: ISCSIPersistentVolumeSource
	    iscsi?: any;
	    // Go type: CinderPersistentVolumeSource
	    cinder?: any;
	    // Go type: CephFSPersistentVolumeSource
	    cephfs?: any;
	    // Go type: FCVolumeSource
	    fc?: any;
	    // Go type: FlockerVolumeSource
	    flocker?: any;
	    // Go type: FlexPersistentVolumeSource
	    flexVolume?: any;
	    // Go type: AzureFilePersistentVolumeSource
	    azureFile?: any;
	    // Go type: VsphereVirtualDiskVolumeSource
	    vsphereVolume?: any;
	    // Go type: QuobyteVolumeSource
	    quobyte?: any;
	    // Go type: AzureDiskVolumeSource
	    azureDisk?: any;
	    // Go type: PhotonPersistentDiskVolumeSource
	    photonPersistentDisk?: any;
	    // Go type: PortworxVolumeSource
	    portworxVolume?: any;
	    // Go type: ScaleIOPersistentVolumeSource
	    scaleIO?: any;
	    // Go type: LocalVolumeSource
	    local?: any;
	    // Go type: StorageOSPersistentVolumeSource
	    storageos?: any;
	    // Go type: CSIPersistentVolumeSource
	    csi?: any;
	    accessModes?: string[];
	    claimRef?: ObjectReference;
	    persistentVolumeReclaimPolicy?: string;
	    storageClassName?: string;
	    mountOptions?: string[];
	    volumeMode?: string;
	    nodeAffinity?: VolumeNodeAffinity;
	    volumeAttributesClassName?: string;
	
	    static createFrom(source: any = {}) {
	        return new PersistentVolumeSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.capacity = this.convertValues(source["capacity"], resource.Quantity, true);
	        this.gcePersistentDisk = this.convertValues(source["gcePersistentDisk"], null);
	        this.awsElasticBlockStore = this.convertValues(source["awsElasticBlockStore"], null);
	        this.hostPath = this.convertValues(source["hostPath"], null);
	        this.glusterfs = this.convertValues(source["glusterfs"], null);
	        this.nfs = this.convertValues(source["nfs"], null);
	        this.rbd = this.convertValues(source["rbd"], null);
	        this.iscsi = this.convertValues(source["iscsi"], null);
	        this.cinder = this.convertValues(source["cinder"], null);
	        this.cephfs = this.convertValues(source["cephfs"], null);
	        this.fc = this.convertValues(source["fc"], null);
	        this.flocker = this.convertValues(source["flocker"], null);
	        this.flexVolume = this.convertValues(source["flexVolume"], null);
	        this.azureFile = this.convertValues(source["azureFile"], null);
	        this.vsphereVolume = this.convertValues(source["vsphereVolume"], null);
	        this.quobyte = this.convertValues(source["quobyte"], null);
	        this.azureDisk = this.convertValues(source["azureDisk"], null);
	        this.photonPersistentDisk = this.convertValues(source["photonPersistentDisk"], null);
	        this.portworxVolume = this.convertValues(source["portworxVolume"], null);
	        this.scaleIO = this.convertValues(source["scaleIO"], null);
	        this.local = this.convertValues(source["local"], null);
	        this.storageos = this.convertValues(source["storageos"], null);
	        this.csi = this.convertValues(source["csi"], null);
	        this.accessModes = source["accessModes"];
	        this.claimRef = this.convertValues(source["claimRef"], ObjectReference);
	        this.persistentVolumeReclaimPolicy = source["persistentVolumeReclaimPolicy"];
	        this.storageClassName = source["storageClassName"];
	        this.mountOptions = source["mountOptions"];
	        this.volumeMode = source["volumeMode"];
	        this.nodeAffinity = this.convertValues(source["nodeAffinity"], VolumeNodeAffinity);
	        this.volumeAttributesClassName = source["volumeAttributesClassName"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PersistentVolume {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: PersistentVolumeSpec;
	    status?: PersistentVolumeStatus;
	
	    static createFrom(source: any = {}) {
	        return new PersistentVolume(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], PersistentVolumeSpec);
	        this.status = this.convertValues(source["status"], PersistentVolumeStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PersistentVolumeClaimCondition {
	    type: string;
	    status: string;
	    lastProbeTime?: Time;
	    lastTransitionTime?: Time;
	    reason?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new PersistentVolumeClaimCondition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.status = source["status"];
	        this.lastProbeTime = this.convertValues(source["lastProbeTime"], Time);
	        this.lastTransitionTime = this.convertValues(source["lastTransitionTime"], Time);
	        this.reason = source["reason"];
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PersistentVolumeClaimStatus {
	    phase?: string;
	    accessModes?: string[];
	    capacity?: Record<string, resource.Quantity>;
	    conditions?: PersistentVolumeClaimCondition[];
	    allocatedResources?: Record<string, resource.Quantity>;
	    allocatedResourceStatuses?: Record<string, string>;
	    currentVolumeAttributesClassName?: string;
	    modifyVolumeStatus?: ModifyVolumeStatus;
	
	    static createFrom(source: any = {}) {
	        return new PersistentVolumeClaimStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.phase = source["phase"];
	        this.accessModes = source["accessModes"];
	        this.capacity = this.convertValues(source["capacity"], resource.Quantity, true);
	        this.conditions = this.convertValues(source["conditions"], PersistentVolumeClaimCondition);
	        this.allocatedResources = this.convertValues(source["allocatedResources"], resource.Quantity, true);
	        this.allocatedResourceStatuses = source["allocatedResourceStatuses"];
	        this.currentVolumeAttributesClassName = source["currentVolumeAttributesClassName"];
	        this.modifyVolumeStatus = this.convertValues(source["modifyVolumeStatus"], ModifyVolumeStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PersistentVolumeClaim {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: PersistentVolumeClaimSpec;
	    status?: PersistentVolumeClaimStatus;
	
	    static createFrom(source: any = {}) {
	        return new PersistentVolumeClaim(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], PersistentVolumeClaimSpec);
	        this.status = this.convertValues(source["status"], PersistentVolumeClaimStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	export class PodExtendedResourceClaimStatus {
	    requestMappings: ContainerExtendedResourceRequest[];
	    resourceClaimName: string;
	
	    static createFrom(source: any = {}) {
	        return new PodExtendedResourceClaimStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requestMappings = this.convertValues(source["requestMappings"], ContainerExtendedResourceRequest);
	        this.resourceClaimName = source["resourceClaimName"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PodResourceClaimStatus {
	    name: string;
	    resourceClaimName?: string;
	
	    static createFrom(source: any = {}) {
	        return new PodResourceClaimStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.resourceClaimName = source["resourceClaimName"];
	    }
	}
	export class PodIP {
	    ip: string;
	
	    static createFrom(source: any = {}) {
	        return new PodIP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	    }
	}
	export class PodCondition {
	    type: string;
	    observedGeneration?: number;
	    status: string;
	    lastProbeTime?: Time;
	    lastTransitionTime?: Time;
	    reason?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new PodCondition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.observedGeneration = source["observedGeneration"];
	        this.status = source["status"];
	        this.lastProbeTime = this.convertValues(source["lastProbeTime"], Time);
	        this.lastTransitionTime = this.convertValues(source["lastTransitionTime"], Time);
	        this.reason = source["reason"];
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PodStatus {
	    observedGeneration?: number;
	    phase?: string;
	    conditions?: PodCondition[];
	    message?: string;
	    reason?: string;
	    nominatedNodeName?: string;
	    hostIP?: string;
	    hostIPs?: HostIP[];
	    podIP?: string;
	    podIPs?: PodIP[];
	    startTime?: Time;
	    initContainerStatuses?: ContainerStatus[];
	    containerStatuses?: ContainerStatus[];
	    qosClass?: string;
	    ephemeralContainerStatuses?: ContainerStatus[];
	    resize?: string;
	    resourceClaimStatuses?: PodResourceClaimStatus[];
	    extendedResourceClaimStatus?: PodExtendedResourceClaimStatus;
	
	    static createFrom(source: any = {}) {
	        return new PodStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.observedGeneration = source["observedGeneration"];
	        this.phase = source["phase"];
	        this.conditions = this.convertValues(source["conditions"], PodCondition);
	        this.message = source["message"];
	        this.reason = source["reason"];
	        this.nominatedNodeName = source["nominatedNodeName"];
	        this.hostIP = source["hostIP"];
	        this.hostIPs = this.convertValues(source["hostIPs"], HostIP);
	        this.podIP = source["podIP"];
	        this.podIPs = this.convertValues(source["podIPs"], PodIP);
	        this.startTime = this.convertValues(source["startTime"], Time);
	        this.initContainerStatuses = this.convertValues(source["initContainerStatuses"], ContainerStatus);
	        this.containerStatuses = this.convertValues(source["containerStatuses"], ContainerStatus);
	        this.qosClass = source["qosClass"];
	        this.ephemeralContainerStatuses = this.convertValues(source["ephemeralContainerStatuses"], ContainerStatus);
	        this.resize = source["resize"];
	        this.resourceClaimStatuses = this.convertValues(source["resourceClaimStatuses"], PodResourceClaimStatus);
	        this.extendedResourceClaimStatus = this.convertValues(source["extendedResourceClaimStatus"], PodExtendedResourceClaimStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Pod {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: PodSpec;
	    status?: PodStatus;
	
	    static createFrom(source: any = {}) {
	        return new Pod(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], PodSpec);
	        this.status = this.convertValues(source["status"], PodStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	export class ReplicaSetCondition {
	    type: string;
	    status: string;
	    lastTransitionTime?: Time;
	    reason?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new ReplicaSetCondition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.status = source["status"];
	        this.lastTransitionTime = this.convertValues(source["lastTransitionTime"], Time);
	        this.reason = source["reason"];
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ReplicaSetStatus {
	    replicas: number;
	    fullyLabeledReplicas?: number;
	    readyReplicas?: number;
	    availableReplicas?: number;
	    terminatingReplicas?: number;
	    observedGeneration?: number;
	    conditions?: ReplicaSetCondition[];
	
	    static createFrom(source: any = {}) {
	        return new ReplicaSetStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.replicas = source["replicas"];
	        this.fullyLabeledReplicas = source["fullyLabeledReplicas"];
	        this.readyReplicas = source["readyReplicas"];
	        this.availableReplicas = source["availableReplicas"];
	        this.terminatingReplicas = source["terminatingReplicas"];
	        this.observedGeneration = source["observedGeneration"];
	        this.conditions = this.convertValues(source["conditions"], ReplicaSetCondition);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ReplicaSetSpec {
	    replicas?: number;
	    minReadySeconds?: number;
	    selector?: LabelSelector;
	    template?: PodTemplateSpec;
	
	    static createFrom(source: any = {}) {
	        return new ReplicaSetSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.replicas = source["replicas"];
	        this.minReadySeconds = source["minReadySeconds"];
	        this.selector = this.convertValues(source["selector"], LabelSelector);
	        this.template = this.convertValues(source["template"], PodTemplateSpec);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ReplicaSet {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: ReplicaSetSpec;
	    status?: ReplicaSetStatus;
	
	    static createFrom(source: any = {}) {
	        return new ReplicaSet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], ReplicaSetSpec);
	        this.status = this.convertValues(source["status"], ReplicaSetStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	
	export class Role {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    rules: PolicyRule[];
	
	    static createFrom(source: any = {}) {
	        return new Role(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.rules = this.convertValues(source["rules"], PolicyRule);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RoleBinding {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    subjects?: Subject[];
	    roleRef: RoleRef;
	
	    static createFrom(source: any = {}) {
	        return new RoleBinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.subjects = this.convertValues(source["subjects"], Subject);
	        this.roleRef = this.convertValues(source["roleRef"], RoleRef);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class RollingUpdateStatefulSetStrategy {
	    partition?: number;
	    maxUnavailable?: intstr.IntOrString;
	
	    static createFrom(source: any = {}) {
	        return new RollingUpdateStatefulSetStrategy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.partition = source["partition"];
	        this.maxUnavailable = this.convertValues(source["maxUnavailable"], intstr.IntOrString);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class Secret {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    immutable?: boolean;
	    data?: Record<string, Array<number>>;
	    stringData?: Record<string, string>;
	    type?: string;
	
	    static createFrom(source: any = {}) {
	        return new Secret(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.immutable = source["immutable"];
	        this.data = source["data"];
	        this.stringData = source["stringData"];
	        this.type = source["type"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	export class ServiceStatus {
	    loadBalancer?: LoadBalancerStatus;
	    conditions?: Condition[];
	
	    static createFrom(source: any = {}) {
	        return new ServiceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.loadBalancer = this.convertValues(source["loadBalancer"], LoadBalancerStatus);
	        this.conditions = this.convertValues(source["conditions"], Condition);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionAffinityConfig {
	    clientIP?: ClientIPConfig;
	
	    static createFrom(source: any = {}) {
	        return new SessionAffinityConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.clientIP = this.convertValues(source["clientIP"], ClientIPConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ServicePort {
	    name?: string;
	    protocol?: string;
	    appProtocol?: string;
	    port: number;
	    targetPort?: intstr.IntOrString;
	    nodePort?: number;
	
	    static createFrom(source: any = {}) {
	        return new ServicePort(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.protocol = source["protocol"];
	        this.appProtocol = source["appProtocol"];
	        this.port = source["port"];
	        this.targetPort = this.convertValues(source["targetPort"], intstr.IntOrString);
	        this.nodePort = source["nodePort"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ServiceSpec {
	    ports?: ServicePort[];
	    selector?: Record<string, string>;
	    clusterIP?: string;
	    clusterIPs?: string[];
	    type?: string;
	    externalIPs?: string[];
	    sessionAffinity?: string;
	    loadBalancerIP?: string;
	    loadBalancerSourceRanges?: string[];
	    externalName?: string;
	    externalTrafficPolicy?: string;
	    healthCheckNodePort?: number;
	    publishNotReadyAddresses?: boolean;
	    sessionAffinityConfig?: SessionAffinityConfig;
	    ipFamilies?: string[];
	    ipFamilyPolicy?: string;
	    allocateLoadBalancerNodePorts?: boolean;
	    loadBalancerClass?: string;
	    internalTrafficPolicy?: string;
	    trafficDistribution?: string;
	
	    static createFrom(source: any = {}) {
	        return new ServiceSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ports = this.convertValues(source["ports"], ServicePort);
	        this.selector = source["selector"];
	        this.clusterIP = source["clusterIP"];
	        this.clusterIPs = source["clusterIPs"];
	        this.type = source["type"];
	        this.externalIPs = source["externalIPs"];
	        this.sessionAffinity = source["sessionAffinity"];
	        this.loadBalancerIP = source["loadBalancerIP"];
	        this.loadBalancerSourceRanges = source["loadBalancerSourceRanges"];
	        this.externalName = source["externalName"];
	        this.externalTrafficPolicy = source["externalTrafficPolicy"];
	        this.healthCheckNodePort = source["healthCheckNodePort"];
	        this.publishNotReadyAddresses = source["publishNotReadyAddresses"];
	        this.sessionAffinityConfig = this.convertValues(source["sessionAffinityConfig"], SessionAffinityConfig);
	        this.ipFamilies = source["ipFamilies"];
	        this.ipFamilyPolicy = source["ipFamilyPolicy"];
	        this.allocateLoadBalancerNodePorts = source["allocateLoadBalancerNodePorts"];
	        this.loadBalancerClass = source["loadBalancerClass"];
	        this.internalTrafficPolicy = source["internalTrafficPolicy"];
	        this.trafficDistribution = source["trafficDistribution"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Service {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: ServiceSpec;
	    status?: ServiceStatus;
	
	    static createFrom(source: any = {}) {
	        return new Service(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], ServiceSpec);
	        this.status = this.convertValues(source["status"], ServiceStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ServiceAccount {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    secrets?: ObjectReference[];
	    imagePullSecrets?: LocalObjectReference[];
	    automountServiceAccountToken?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ServiceAccount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.secrets = this.convertValues(source["secrets"], ObjectReference);
	        this.imagePullSecrets = this.convertValues(source["imagePullSecrets"], LocalObjectReference);
	        this.automountServiceAccountToken = source["automountServiceAccountToken"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	export class StatefulSetCondition {
	    type: string;
	    status: string;
	    lastTransitionTime?: Time;
	    reason?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new StatefulSetCondition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.status = source["status"];
	        this.lastTransitionTime = this.convertValues(source["lastTransitionTime"], Time);
	        this.reason = source["reason"];
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StatefulSetStatus {
	    observedGeneration?: number;
	    replicas: number;
	    readyReplicas?: number;
	    currentReplicas?: number;
	    updatedReplicas?: number;
	    currentRevision?: string;
	    updateRevision?: string;
	    collisionCount?: number;
	    conditions?: StatefulSetCondition[];
	    availableReplicas: number;
	
	    static createFrom(source: any = {}) {
	        return new StatefulSetStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.observedGeneration = source["observedGeneration"];
	        this.replicas = source["replicas"];
	        this.readyReplicas = source["readyReplicas"];
	        this.currentReplicas = source["currentReplicas"];
	        this.updatedReplicas = source["updatedReplicas"];
	        this.currentRevision = source["currentRevision"];
	        this.updateRevision = source["updateRevision"];
	        this.collisionCount = source["collisionCount"];
	        this.conditions = this.convertValues(source["conditions"], StatefulSetCondition);
	        this.availableReplicas = source["availableReplicas"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StatefulSetOrdinals {
	    start: number;
	
	    static createFrom(source: any = {}) {
	        return new StatefulSetOrdinals(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start = source["start"];
	    }
	}
	export class StatefulSetPersistentVolumeClaimRetentionPolicy {
	    whenDeleted?: string;
	    whenScaled?: string;
	
	    static createFrom(source: any = {}) {
	        return new StatefulSetPersistentVolumeClaimRetentionPolicy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.whenDeleted = source["whenDeleted"];
	        this.whenScaled = source["whenScaled"];
	    }
	}
	export class StatefulSetUpdateStrategy {
	    type?: string;
	    rollingUpdate?: RollingUpdateStatefulSetStrategy;
	
	    static createFrom(source: any = {}) {
	        return new StatefulSetUpdateStrategy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.rollingUpdate = this.convertValues(source["rollingUpdate"], RollingUpdateStatefulSetStrategy);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StatefulSetSpec {
	    replicas?: number;
	    selector?: LabelSelector;
	    template: PodTemplateSpec;
	    volumeClaimTemplates?: PersistentVolumeClaim[];
	    serviceName: string;
	    podManagementPolicy?: string;
	    updateStrategy?: StatefulSetUpdateStrategy;
	    revisionHistoryLimit?: number;
	    minReadySeconds?: number;
	    persistentVolumeClaimRetentionPolicy?: StatefulSetPersistentVolumeClaimRetentionPolicy;
	    ordinals?: StatefulSetOrdinals;
	
	    static createFrom(source: any = {}) {
	        return new StatefulSetSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.replicas = source["replicas"];
	        this.selector = this.convertValues(source["selector"], LabelSelector);
	        this.template = this.convertValues(source["template"], PodTemplateSpec);
	        this.volumeClaimTemplates = this.convertValues(source["volumeClaimTemplates"], PersistentVolumeClaim);
	        this.serviceName = source["serviceName"];
	        this.podManagementPolicy = source["podManagementPolicy"];
	        this.updateStrategy = this.convertValues(source["updateStrategy"], StatefulSetUpdateStrategy);
	        this.revisionHistoryLimit = source["revisionHistoryLimit"];
	        this.minReadySeconds = source["minReadySeconds"];
	        this.persistentVolumeClaimRetentionPolicy = this.convertValues(source["persistentVolumeClaimRetentionPolicy"], StatefulSetPersistentVolumeClaimRetentionPolicy);
	        this.ordinals = this.convertValues(source["ordinals"], StatefulSetOrdinals);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StatefulSet {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    spec?: StatefulSetSpec;
	    status?: StatefulSetStatus;
	
	    static createFrom(source: any = {}) {
	        return new StatefulSet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.spec = this.convertValues(source["spec"], StatefulSetSpec);
	        this.status = this.convertValues(source["status"], StatefulSetStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	export class TopologySelectorLabelRequirement {
	    key: string;
	    values: string[];
	
	    static createFrom(source: any = {}) {
	        return new TopologySelectorLabelRequirement(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.values = source["values"];
	    }
	}
	export class TopologySelectorTerm {
	    matchLabelExpressions?: TopologySelectorLabelRequirement[];
	
	    static createFrom(source: any = {}) {
	        return new TopologySelectorTerm(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.matchLabelExpressions = this.convertValues(source["matchLabelExpressions"], TopologySelectorLabelRequirement);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StorageClass {
	    kind?: string;
	    apiVersion?: string;
	    name?: string;
	    generateName?: string;
	    namespace?: string;
	    selfLink?: string;
	    uid?: string;
	    resourceVersion?: string;
	    generation?: number;
	    creationTimestamp?: Time;
	    deletionTimestamp?: Time;
	    deletionGracePeriodSeconds?: number;
	    labels?: Record<string, string>;
	    annotations?: Record<string, string>;
	    ownerReferences?: OwnerReference[];
	    finalizers?: string[];
	    managedFields?: ManagedFieldsEntry[];
	    provisioner: string;
	    parameters?: Record<string, string>;
	    reclaimPolicy?: string;
	    mountOptions?: string[];
	    allowVolumeExpansion?: boolean;
	    volumeBindingMode?: string;
	    allowedTopologies?: TopologySelectorTerm[];
	
	    static createFrom(source: any = {}) {
	        return new StorageClass(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.generateName = source["generateName"];
	        this.namespace = source["namespace"];
	        this.selfLink = source["selfLink"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.generation = source["generation"];
	        this.creationTimestamp = this.convertValues(source["creationTimestamp"], Time);
	        this.deletionTimestamp = this.convertValues(source["deletionTimestamp"], Time);
	        this.deletionGracePeriodSeconds = source["deletionGracePeriodSeconds"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.ownerReferences = this.convertValues(source["ownerReferences"], OwnerReference);
	        this.finalizers = source["finalizers"];
	        this.managedFields = this.convertValues(source["managedFields"], ManagedFieldsEntry);
	        this.provisioner = source["provisioner"];
	        this.parameters = source["parameters"];
	        this.reclaimPolicy = source["reclaimPolicy"];
	        this.mountOptions = source["mountOptions"];
	        this.allowVolumeExpansion = source["allowVolumeExpansion"];
	        this.volumeBindingMode = source["volumeBindingMode"];
	        this.allowedTopologies = this.convertValues(source["allowedTopologies"], TopologySelectorTerm);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	

}

