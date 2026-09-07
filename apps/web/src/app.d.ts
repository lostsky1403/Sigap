import type { SupabaseClient } from '@supabase/ssr';
import type { AuthSessionLike } from './lib/server/auth';

declare global {
	namespace App {
		interface Locals {
			supabase: SupabaseClient | null;
			session: AuthSessionLike | null;
		}
	}
}

export {};
