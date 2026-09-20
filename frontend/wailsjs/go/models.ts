export namespace store {
	
	export class Connection {
	    id: number;
	    label: string;
	    host: string;
	    sshPort: number;
	    username: string;
	    remoteBindPort: number;
	    localPort: number;
	    password?: string;
	    hasPassword: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Connection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.host = source["host"];
	        this.sshPort = source["sshPort"];
	        this.username = source["username"];
	        this.remoteBindPort = source["remoteBindPort"];
	        this.localPort = source["localPort"];
	        this.password = source["password"];
	        this.hasPassword = source["hasPassword"];
	    }
	}

}

export namespace tunnel {
	
	export class PendingHostKey {
	    host: string;
	    keyType: string;
	    fingerprint: string;
	
	    static createFrom(source: any = {}) {
	        return new PendingHostKey(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.keyType = source["keyType"];
	        this.fingerprint = source["fingerprint"];
	    }
	}
	export class Status {
	    id: number;
	    state: string;
	    error: string;
	    streams: number;
	    since: string;
	    pendingHostKey?: PendingHostKey;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.state = source["state"];
	        this.error = source["error"];
	        this.streams = source["streams"];
	        this.since = source["since"];
	        this.pendingHostKey = this.convertValues(source["pendingHostKey"], PendingHostKey);
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

