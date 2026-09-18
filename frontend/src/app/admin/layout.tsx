"use client";

import { useEffect, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { getSession } from "@/lib/api";
import AdminSidebar from "@/components/AdminSidebar";
import { AdminSessionProvider, type AdminSession } from "@/context/AdminSessionContext";

export default function AdminLayout({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [session, setSession] = useState<AdminSession | null>(null);

  useEffect(() => {
    getSession()
      .then((data) => setSession({ email: data.email, isSuperAdmin: Boolean(data.is_super_admin) }))
      .catch(() => router.replace("/login"));
  }, [router]);

  if (!session) {
    return (
      <main className="min-h-screen bg-[#f2f0ec] flex items-center justify-center">
        <p className="text-xs text-gray-400">Loading...</p>
      </main>
    );
  }

  return (
    <AdminSessionProvider value={session}>
      <main className="min-h-screen bg-[#f2f0ec] flex">
        <AdminSidebar email={session.email} />
        <div className="flex-1 p-6">{children}</div>
      </main>
    </AdminSessionProvider>
  );
}
