declare module "*/wailsjs/go/main/App" {
  export function GetAuthStatus(): Promise<Record<string, any>>;
  export function Login(countryCode: string, phoneNumber: string, code: string): Promise<void>;
  export function Logout(): Promise<void>;
  export function SendLoginCode(countryCode: string, phoneNumber: string): Promise<Record<string, any>>;
  export function RefreshToken(): Promise<Record<string, any>>;
  export function GetProfile(): Promise<any>;
  export function UpdateProfile(profile: any): Promise<void>;
  export function StartServer(): Promise<void>;
  export function StopServer(): Promise<void>;
  export function IsRunning(): Promise<boolean>;
  export function GetServerInfo(): Promise<Record<string, any>>;
  export function GetLLMConfig(): Promise<Record<string, any>>;
}

declare module "*/wailsjs/runtime/runtime" {
  export function EventsOn(eventName: string, callback: (...data: any[]) => void): () => void;
  export function EventsOff(eventName: string): void;
}
