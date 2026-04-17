import { createRoot } from "react-dom/client";
import "./index.css";
import App from "./App";

// NOTE: StrictMode is intentionally disabled. In React 19 StrictMode
// double-invokes effects in dev mode, which causes the ChatPage to open
// two WebSocket connections (the second one ends up using a stale admin
// token, flooding the server with "invalid token" errors).
createRoot(document.getElementById("root")!).render(<App />);
