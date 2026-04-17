// Type declarations for Wails runtime bindings.
// In production, Wails generates these; in dev mode we declare them manually.

declare module "*/wailsjs/go/main/App" {
  // Auth
  export function GetAuthStatus(): Promise<Record<string, any>>;
  export function Logout(): Promise<void>;
  export function RequestDesktopLoginCode(
    countryCode: string,
    phoneNumber: string
  ): Promise<Record<string, any>>;
  export function VerifyDesktopLoginCode(
    countryCode: string,
    phoneNumber: string,
    verificationCode: string
  ): Promise<void>;

  // Server lifecycle
  export function StartServer(): Promise<void>;
  export function StopServer(): Promise<void>;
  export function IsRunning(): Promise<boolean>;
  export function GetServerInfo(): Promise<Record<string, any>>;
  export function GetLLMConfig(): Promise<Record<string, any>>;
}

declare module "*/wailsjs/runtime/runtime" {
  export function EventsOn(
    eventName: string,
    callback: (...data: any[]) => void
  ): () => void;
  export function EventsOff(eventName: string): void;
}
