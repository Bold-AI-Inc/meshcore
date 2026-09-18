"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { logout } from "@/lib/api";

interface AdminSidebarProps {
  email: string;
}

const NAV_ITEMS = [
  { href: "/admin/dashboard", label: "Dashboard" },
  { href: "/admin/logs", label: "Logs" },
  { href: "/admin/users", label: "Users" },
  { href: "/admin/admins", label: "Admins" },
  { href: "/admin/providers", label: "Providers" },
  { href: "/admin", label: "Profile" },
];

export default function AdminSidebar({ email }: AdminSidebarProps) {
  const router = useRouter();
  const pathname = usePathname();

  async function handleLogout() {
    try {
      await logout();
    } finally {
      router.replace("/login");
    }
  }

  return (
    <aside className="w-48 shrink-0 bg-white rounded-lg shadow-sm p-4 m-3 mr-0 flex flex-col justify-between">
      <div>
        <div className="text-sm font-medium mb-6">mesh</div>
        <nav className="space-y-1">
          {NAV_ITEMS.map((item) => {
            const active = pathname === item.href;
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`block rounded-md text-xs font-medium px-3 py-1.5 ${
                  active ? "bg-black text-white" : "text-gray-600 hover:bg-gray-50"
                }`}
              >
                {item.label}
              </Link>
            );
          })}
        </nav>
      </div>
      <div className="space-y-2">
        <div className="border-t border-gray-100 pt-3">
          <p className="text-[11px] text-gray-400 mb-0.5">Signed in as</p>
          <p className="text-xs truncate">{email}</p>
        </div>
        <button
          onClick={handleLogout}
          className="w-full rounded-md border border-gray-300 text-xs font-medium px-3 py-1.5 hover:bg-gray-50"
        >
          Log out
        </button>
      </div>
    </aside>
  );
}
