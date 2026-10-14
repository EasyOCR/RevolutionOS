export namespace main {
	
	export class Account {
	    name: string;
	    username: string;
	    running: boolean;
	    display: string;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.username = source["username"];
	        this.running = source["running"];
	        this.display = source["display"];
	    }
	}
	export class Instance {
	    name: string;
	    running: boolean;
	    display: string;
	
	    static createFrom(source: any = {}) {
	        return new Instance(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.running = source["running"];
	        this.display = source["display"];
	    }
	}

}

