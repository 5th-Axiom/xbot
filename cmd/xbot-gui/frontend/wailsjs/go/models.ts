export namespace config {
	
	export class AdminConfig {
	    chat_id: string;
	    token: string;
	
	    static createFrom(source: any = {}) {
	        return new AdminConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chat_id = source["chat_id"];
	        this.token = source["token"];
	    }
	}
	export class AgentConfig {
	    max_iterations: number;
	    max_concurrency: number;
	    memory_provider: string;
	    work_dir: string;
	    prompt_file: string;
	    single_user: boolean;
	    mcp_inactivity_timeout: number;
	    mcp_cleanup_interval: number;
	    session_cache_timeout: number;
	    context_mode: string;
	    enable_auto_compress?: boolean;
	    max_context_tokens: number;
	    compression_threshold: number;
	    purge_old_messages: boolean;
	    max_sub_agent_depth: number;
	    llm_retry_attempts: number;
	    llm_retry_delay: number;
	    llm_retry_max_delay: number;
	    llm_retry_timeout: number;
	
	    static createFrom(source: any = {}) {
	        return new AgentConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.max_iterations = source["max_iterations"];
	        this.max_concurrency = source["max_concurrency"];
	        this.memory_provider = source["memory_provider"];
	        this.work_dir = source["work_dir"];
	        this.prompt_file = source["prompt_file"];
	        this.single_user = source["single_user"];
	        this.mcp_inactivity_timeout = source["mcp_inactivity_timeout"];
	        this.mcp_cleanup_interval = source["mcp_cleanup_interval"];
	        this.session_cache_timeout = source["session_cache_timeout"];
	        this.context_mode = source["context_mode"];
	        this.enable_auto_compress = source["enable_auto_compress"];
	        this.max_context_tokens = source["max_context_tokens"];
	        this.compression_threshold = source["compression_threshold"];
	        this.purge_old_messages = source["purge_old_messages"];
	        this.max_sub_agent_depth = source["max_sub_agent_depth"];
	        this.llm_retry_attempts = source["llm_retry_attempts"];
	        this.llm_retry_delay = source["llm_retry_delay"];
	        this.llm_retry_max_delay = source["llm_retry_max_delay"];
	        this.llm_retry_timeout = source["llm_retry_timeout"];
	    }
	}
	export class SubscriptionConfig {
	    id: string;
	    name: string;
	    provider: string;
	    base_url: string;
	    api_key: string;
	    model: string;
	    max_output_tokens?: number;
	    thinking_mode?: string;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SubscriptionConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.provider = source["provider"];
	        this.base_url = source["base_url"];
	        this.api_key = source["api_key"];
	        this.model = source["model"];
	        this.max_output_tokens = source["max_output_tokens"];
	        this.thinking_mode = source["thinking_mode"];
	        this.active = source["active"];
	    }
	}
	export class OSSConfig {
	    provider: string;
	    qiniu_access_key: string;
	    qiniu_secret_key: string;
	    qiniu_bucket: string;
	    qiniu_domain: string;
	    qiniu_region: string;
	
	    static createFrom(source: any = {}) {
	        return new OSSConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.qiniu_access_key = source["qiniu_access_key"];
	        this.qiniu_secret_key = source["qiniu_secret_key"];
	        this.qiniu_bucket = source["qiniu_bucket"];
	        this.qiniu_domain = source["qiniu_domain"];
	        this.qiniu_region = source["qiniu_region"];
	    }
	}
	export class EventWebhookConfig {
	    enable: boolean;
	    host: string;
	    port: number;
	    base_url: string;
	    max_body_size: number;
	    rate_limit: number;
	
	    static createFrom(source: any = {}) {
	        return new EventWebhookConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enable = source["enable"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.base_url = source["base_url"];
	        this.max_body_size = source["max_body_size"];
	        this.rate_limit = source["rate_limit"];
	    }
	}
	export class WebConfig {
	    enable: boolean;
	    host: string;
	    port: number;
	    static_dir: string;
	    upload_dir: string;
	    persona_isolation: boolean;
	    invite_only: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WebConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enable = source["enable"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.static_dir = source["static_dir"];
	        this.upload_dir = source["upload_dir"];
	        this.persona_isolation = source["persona_isolation"];
	        this.invite_only = source["invite_only"];
	    }
	}
	export class StartupNotifyConfig {
	    channel: string;
	    chat_id: string;
	
	    static createFrom(source: any = {}) {
	        return new StartupNotifyConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.channel = source["channel"];
	        this.chat_id = source["chat_id"];
	    }
	}
	export class SandboxConfig {
	    mode: string;
	    remote_mode: string;
	    docker_image: string;
	    host_work_dir: string;
	    idle_timeout: number;
	    ws_port: number;
	    auth_token: string;
	    public_url: string;
	
	    static createFrom(source: any = {}) {
	        return new SandboxConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.remote_mode = source["remote_mode"];
	        this.docker_image = source["docker_image"];
	        this.host_work_dir = source["host_work_dir"];
	        this.idle_timeout = source["idle_timeout"];
	        this.ws_port = source["ws_port"];
	        this.auth_token = source["auth_token"];
	        this.public_url = source["public_url"];
	    }
	}
	export class OAuthConfig {
	    enable: boolean;
	    host: string;
	    port: number;
	    base_url: string;
	
	    static createFrom(source: any = {}) {
	        return new OAuthConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enable = source["enable"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.base_url = source["base_url"];
	    }
	}
	export class NapCatConfig {
	    enabled: boolean;
	    ws_url: string;
	    token: string;
	    allow_from: string[];
	
	    static createFrom(source: any = {}) {
	        return new NapCatConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.ws_url = source["ws_url"];
	        this.token = source["token"];
	        this.allow_from = source["allow_from"];
	    }
	}
	export class QQConfig {
	    enabled: boolean;
	    app_id: string;
	    client_secret: string;
	    allow_from: string[];
	
	    static createFrom(source: any = {}) {
	        return new QQConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.app_id = source["app_id"];
	        this.client_secret = source["client_secret"];
	        this.allow_from = source["allow_from"];
	    }
	}
	export class FeishuConfig {
	    enabled: boolean;
	    app_id: string;
	    app_secret: string;
	    encrypt_key: string;
	    verification_token: string;
	    allow_from: string[];
	    domain: string;
	
	    static createFrom(source: any = {}) {
	        return new FeishuConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.app_id = source["app_id"];
	        this.app_secret = source["app_secret"];
	        this.encrypt_key = source["encrypt_key"];
	        this.verification_token = source["verification_token"];
	        this.allow_from = source["allow_from"];
	        this.domain = source["domain"];
	    }
	}
	export class PProfConfig {
	    enable: boolean;
	    host: string;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new PProfConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enable = source["enable"];
	        this.host = source["host"];
	        this.port = source["port"];
	    }
	}
	export class LogConfig {
	    level: string;
	    format: string;
	
	    static createFrom(source: any = {}) {
	        return new LogConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.format = source["format"];
	    }
	}
	export class EmbeddingConfig {
	    provider: string;
	    base_url: string;
	    api_key: string;
	    model: string;
	    max_tokens: number;
	
	    static createFrom(source: any = {}) {
	        return new EmbeddingConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.base_url = source["base_url"];
	        this.api_key = source["api_key"];
	        this.model = source["model"];
	        this.max_tokens = source["max_tokens"];
	    }
	}
	export class LLMConfig {
	    provider: string;
	    base_url: string;
	    api_key: string;
	    model: string;
	    vanguard_model?: string;
	    balance_model?: string;
	    swift_model?: string;
	    max_output_tokens?: number;
	    thinking_mode?: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.base_url = source["base_url"];
	        this.api_key = source["api_key"];
	        this.model = source["model"];
	        this.vanguard_model = source["vanguard_model"];
	        this.balance_model = source["balance_model"];
	        this.swift_model = source["swift_model"];
	        this.max_output_tokens = source["max_output_tokens"];
	        this.thinking_mode = source["thinking_mode"];
	    }
	}
	export class ServerConfig {
	    host: string;
	    port: number;
	    read_timeout: number;
	    write_timeout: number;
	
	    static createFrom(source: any = {}) {
	        return new ServerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.read_timeout = source["read_timeout"];
	        this.write_timeout = source["write_timeout"];
	    }
	}
	export class Config {
	    server: ServerConfig;
	    llm: LLMConfig;
	    embedding: EmbeddingConfig;
	    log: LogConfig;
	    pprof: PProfConfig;
	    feishu: FeishuConfig;
	    qq: QQConfig;
	    napcat: NapCatConfig;
	    agent: AgentConfig;
	    oauth: OAuthConfig;
	    sandbox: SandboxConfig;
	    startup_notify: StartupNotifyConfig;
	    admin: AdminConfig;
	    web: WebConfig;
	    event_webhook: EventWebhookConfig;
	    oss: OSSConfig;
	    tavily_api_key: string;
	    subscriptions?: SubscriptionConfig[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server = this.convertValues(source["server"], ServerConfig);
	        this.llm = this.convertValues(source["llm"], LLMConfig);
	        this.embedding = this.convertValues(source["embedding"], EmbeddingConfig);
	        this.log = this.convertValues(source["log"], LogConfig);
	        this.pprof = this.convertValues(source["pprof"], PProfConfig);
	        this.feishu = this.convertValues(source["feishu"], FeishuConfig);
	        this.qq = this.convertValues(source["qq"], QQConfig);
	        this.napcat = this.convertValues(source["napcat"], NapCatConfig);
	        this.agent = this.convertValues(source["agent"], AgentConfig);
	        this.oauth = this.convertValues(source["oauth"], OAuthConfig);
	        this.sandbox = this.convertValues(source["sandbox"], SandboxConfig);
	        this.startup_notify = this.convertValues(source["startup_notify"], StartupNotifyConfig);
	        this.admin = this.convertValues(source["admin"], AdminConfig);
	        this.web = this.convertValues(source["web"], WebConfig);
	        this.event_webhook = this.convertValues(source["event_webhook"], EventWebhookConfig);
	        this.oss = this.convertValues(source["oss"], OSSConfig);
	        this.tavily_api_key = source["tavily_api_key"];
	        this.subscriptions = this.convertValues(source["subscriptions"], SubscriptionConfig);
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

export namespace main {
	
	export class AgentProfile {
	    uid?: string;
	    name: string;
	    bio: string;
	    tags: string[];
	    goals: string;
	    recent_context: string;
	    looking_for: string;
	    city: string;
	    status?: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uid = source["uid"];
	        this.name = source["name"];
	        this.bio = source["bio"];
	        this.tags = source["tags"];
	        this.goals = source["goals"];
	        this.recent_context = source["recent_context"];
	        this.looking_for = source["looking_for"];
	        this.city = source["city"];
	        this.status = source["status"];
	    }
	}
	export class LLMConfigSpec {
	    provider: string;
	    base_url: string;
	    api_key: string;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMConfigSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.base_url = source["base_url"];
	        this.api_key = source["api_key"];
	        this.model = source["model"];
	    }
	}

}

