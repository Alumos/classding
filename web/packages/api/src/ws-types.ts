// Generated from api/ws/frame.schema.json. Do not edit.
export type paths = Record<string, never>;
export type webhooks = Record<string, never>;
export interface components {
    schemas: {
        /** ClassDing WS Frame */
        Frame: {
            /** @constant */
            t: "hello";
            id?: string;
            seq?: number;
            ts?: number;
            d: {
                token: string;
                last_seq?: number;
                client_version?: string;
                os?: string;
                pending_ids?: string[];
            };
        } | {
            /** @constant */
            t: "ack";
            id?: string;
            seq?: number;
            ts?: number;
            d: {
                message_id: string;
                /** @enum {string} */
                status: "delivered" | "playing" | "played" | "dismissed" | "failed";
                err?: string;
            };
        } | {
            /** @constant */
            t: "welcome";
            id?: string;
            seq?: number;
            ts?: number;
            d: {
                class?: {
                    id: string;
                    name: string;
                };
                device?: {
                    id: string;
                    class_id: string;
                    name: string;
                };
                volume?: number;
                sequence?: number;
                remote_config?: {
                    server_url: string;
                    revision: string;
                } | null;
                server_time: number;
                recalled_ids?: string[];
            };
        } | {
            /** @constant */
            t: "notify";
            id?: string;
            seq?: number;
            ts?: number;
            d: {
                message_id: string;
                style: string;
                text: string;
                student_name: string;
                signature: string;
                class_name: string;
                audio_url: string;
                chime: boolean;
                repeat: number;
                display_sec: number;
                volume: number;
                /** Format: date-time */
                sent_at: string;
                /** Format: date-time */
                expires_at: string;
            };
        } | {
            /** @constant */
            t: "recall";
            id?: string;
            seq?: number;
            ts?: number;
            d: {
                message_id: string;
            };
        } | {
            /** @constant */
            t: "tool";
            id?: string;
            seq?: number;
            ts?: number;
            d: {
                /** @enum {string} */
                name: "countdown" | "rollcall" | "stop";
                params: Record<string, never>;
            };
        } | {
            /** @constant */
            t: "config";
            id?: string;
            seq?: number;
            ts?: number;
            d: {
                volume?: number;
                remote_config?: {
                    server_url: string;
                    revision: string;
                } | null;
            };
        } | {
            /** @constant */
            t: "unbind";
            id?: string;
            seq?: number;
            ts?: number;
            d: Record<string, never>;
        } | {
            /** @constant */
            t: "ping";
            id?: string;
            seq?: number;
            ts?: number;
            d: Record<string, never>;
        } | {
            /** @constant */
            t: "pong";
            id?: string;
            seq?: number;
            ts?: number;
            d: {
                server_time: number;
            };
        } | {
            /** @constant */
            t: "receipt";
            id?: string;
            seq?: number;
            ts?: number;
            d: {
                message_id: string;
                class_id: string;
                device_id: string;
                status: string;
                /** Format: date-time */
                at: string;
            };
        } | {
            /** @constant */
            t: "presence";
            id?: string;
            seq?: number;
            ts?: number;
            d: {
                class_id: string;
                device_id: string;
                online: boolean;
            };
        };
    };
    responses: never;
    parameters: never;
    requestBodies: never;
    headers: never;
    pathItems: never;
}
export type $defs = Record<string, never>;
export type operations = Record<string, never>;
