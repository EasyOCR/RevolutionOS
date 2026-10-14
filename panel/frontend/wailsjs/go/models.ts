export namespace main {
	
	export class AppEntry {
	    name: string;
	    exec: string;
	    categories: string;
	
	    static createFrom(source: any = {}) {
	        return new AppEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.exec = source["exec"];
	        this.categories = source["categories"];
	    }
	}
	export class PlaceEntry {
	    name: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new PlaceEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	    }
	}
	export class TrayItem {
	    service: string;
	    path: string;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new TrayItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.service = source["service"];
	        this.path = source["path"];
	        this.title = source["title"];
	    }
	}
	export class WindowEntry {
	    id: number;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new WindowEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	    }
	}

}

