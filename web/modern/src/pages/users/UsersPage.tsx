import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { EnhancedDataTable } from "@/components/ui/enhanced-data-table";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { useNotifications } from "@/components/ui/notifications";
import { ResponsiveActionGroup } from "@/components/ui/responsive-action-group";
import { ResponsivePageContainer } from "@/components/ui/responsive-container";
import { type SearchOption } from "@/components/ui/searchable-dropdown";
import { TimestampDisplay } from "@/components/ui/timestamp";
import { usePageSize, STORAGE_KEYS } from "@/hooks/usePersistentState";
import { useResponsive } from "@/hooks/useResponsive";
import { api } from "@/lib/api";
import { cn, renderQuota } from "@/lib/utils";
import { zodResolver } from "@hookform/resolvers/zod";
import type { ColumnDef } from "@tanstack/react-table";
import { Ban, CheckCircle, CreditCard, Settings, Trash2 } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { useNavigate, useSearchParams } from "react-router-dom";
import * as z from "zod";

interface UserRow {
  id: number;
  username: string;
  display_name?: string;
  role: number;
  status: number;
  email?: string;
  quota: number;
  used_quota: number;
  group: string;
  created_at?: number;
  updated_at?: number;
}

export function UsersPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { isMobile } = useResponsive();
  const { notify } = useNotifications();
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`users.page.${key}`, { defaultValue, ...options }),
    [t]
  );
  const [data, setData] = useState<UserRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [pageIndex, setPageIndex] = useState(
    Math.max(0, parseInt(searchParams.get("p") || "1") - 1)
  );
  const [pageSize, setPageSize] = usePageSize(STORAGE_KEYS.PAGE_SIZE);
  const [total, setTotal] = useState(0);
  const [searchKeyword, setSearchKeyword] = useState("");
  const [searchOptions, setSearchOptions] = useState<SearchOption[]>([]);
  const [searchLoading, setSearchLoading] = useState(false);
  const [sortBy, setSortBy] = useState("");
  const [sortOrder, setSortOrder] = useState<"asc" | "desc">("desc");
  const [openCreate, setOpenCreate] = useState(false);
  const [openTopup, setOpenTopup] = useState<{
    open: boolean;
    userId?: number;
    username?: string;
  }>({ open: false });
  const mounted = useRef(false);
  const getRoleLabel = useCallback(
    (role: number) => {
      if (role >= 10) {
        return tr("table.role.super_admin", "Super Admin");
      }
      if (role === 1) {
        return tr("table.role.admin", "Admin");
      }
      return tr("table.role.user", "User");
    },
    [tr]
  );
  const getStatusLabel = useCallback(
    (status: number) =>
      status === 1
        ? tr("table.status.enabled", "Enabled")
        : tr("table.status.disabled", "Disabled"),
    [tr]
  );
  const formatRemainingQuota = useCallback(
    (quota: number) => {
      if (quota === -1) {
        return tr("table.quota.unlimited", "Unlimited");
      }
      return renderQuota(quota);
    },
    [tr]
  );

  const load = async (p = 0, size = pageSize) => {
    setLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      let url = `/api/user/?p=${p}&size=${size}`;
      if (sortBy) url += `&sort=${sortBy}&order=${sortOrder}`;
      const res = await api.get(url);
      const { success, data, total } = res.data;
      if (success) {
        setData(data);
        setTotal(total || data.length);
        setPageIndex(p);
        setPageSize(size);
      }
    } catch (error) {
      const message =
        (error as any)?.response?.data?.message ||
        tr("notifications.load_failed_message", "Failed to load users.");
      notify({
        type: "error",
        title: tr("notifications.load_failed_title", "Access denied"),
        message,
      });
    } finally {
      setLoading(false);
    }
  };

  const searchUsers = async (query: string) => {
    if (!query.trim()) {
      setSearchOptions([]);
      return;
    }

    setSearchLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      const res = await api.get(
        `/api/user/search?keyword=${encodeURIComponent(query)}`
      );
      const { success, data } = res.data;
      if (success && Array.isArray(data)) {
        const options: SearchOption[] = data.map((user: UserRow) => ({
          key: user.id.toString(),
          value: user.username,
          text: user.username,
          content: (
            <div className="flex flex-col">
              <div className="font-medium">{user.username}</div>
              <div className="text-sm text-muted-foreground flex flex-wrap gap-2">
                <span>
                  {tr("search.id_label", "ID: {{id}}", { id: user.id })}
                </span>
                <span>
                  {tr("search.role_label", "Role: {{role}}", {
                    role: getRoleLabel(user.role),
                  })}
                </span>
                <span>
                  {tr("search.status_label", "Status: {{status}}", {
                    status: getStatusLabel(user.status),
                  })}
                </span>
              </div>
            </div>
          ),
        }));
        setSearchOptions(options);
      }
    } catch (error) {
      console.error("Search failed:", error);
    } finally {
      setSearchLoading(false);
    }
  };

  useEffect(() => {
    if (!mounted.current) {
      mounted.current = true;
      load(pageIndex, pageSize);
      return;
    }
    if (searchKeyword.trim()) {
      search();
    } else {
      load(0, pageSize);
    }
  }, [sortBy, sortOrder]);

  const search = async () => {
    setLoading(true);
    try {
      if (!searchKeyword.trim()) return load(0, pageSize);
      // Unified API call - complete URL with /api prefix
      let url = `/api/user/search?keyword=${encodeURIComponent(searchKeyword)}`;
      if (sortBy) url += `&sort=${sortBy}&order=${sortOrder}`;
      url += `&size=${pageSize}`;
      const res = await api.get(url);
      const { success, data } = res.data;
      if (success) {
        setData(data);
        setPageIndex(0);
      }
    } catch (error) {
      const message =
        (error as any)?.response?.data?.message ||
        tr("notifications.search_failed_message", "Search failed.");
      notify({
        type: "error",
        title: tr("notifications.search_failed_title", "Search failed"),
        message,
      });
    } finally {
      setLoading(false);
    }
  };

  const columns: ColumnDef<UserRow>[] = [
    { header: tr("columns.id", "ID"), accessorKey: "id" },
    { header: tr("columns.username", "Username"), accessorKey: "username" },
    {
      header: tr("columns.display_name", "Display Name"),
      accessorKey: "display_name",
    },
    {
      header: tr("columns.role", "Role"),
      cell: ({ row }) => getRoleLabel(row.original.role),
    },
    {
      header: tr("columns.status", "Status"),
      cell: ({ row }) => getStatusLabel(row.original.status),
    },
    { header: tr("columns.group", "Group"), accessorKey: "group" },
    {
      header: tr("columns.used_quota", "Used Quota"),
      accessorKey: "used_quota",
      cell: ({ row }) => {
        const quotaLabel = renderQuota(row.original.used_quota || 0);
        return (
          <span
            className="font-mono text-sm"
            title={tr("table.used_quota_title", "Used: {{quota}}", {
              quota: quotaLabel,
            })}
          >
            {quotaLabel}
          </span>
        );
      },
    },
    {
      header: tr("columns.remaining_quota", "Remaining Quota"),
      accessorKey: "quota",
      cell: ({ row }) => {
        const quotaLabel = formatRemainingQuota(row.original.quota);
        return (
          <span
            className="font-mono text-sm"
            title={tr("table.remaining_quota_title", "Remaining: {{quota}}", {
              quota: quotaLabel,
            })}
          >
            {row.original.quota === -1 ? (
              <span className="text-green-600 font-semibold">{quotaLabel}</span>
            ) : (
              quotaLabel
            )}
          </span>
        );
      },
    },
    {
      header: tr("columns.register_time", "Register Time"),
      accessorKey: "created_at",
      cell: ({ row }) => {
        // Use created_at if valid, otherwise fallback to updated_at
        // Note: User timestamps are stored in milliseconds, convert to seconds for display
        const timestampMs =
          row.original.created_at && row.original.created_at > 0
            ? row.original.created_at
            : row.original.updated_at;
        const timestampSec =
          timestampMs && timestampMs > 0
            ? Math.floor(timestampMs / 1000)
            : undefined;
        return (
          <TimestampDisplay
            timestamp={timestampSec}
            className="text-sm"
            fallback="-"
          />
        );
      },
    },
    {
      header: tr("columns.actions", "Actions"),
      cell: ({ row }) => (
        <ResponsiveActionGroup justify="start">
          <Button
            variant="outline"
            size="sm"
            onClick={() => navigate(`/users/edit/${row.original.id}`)}
          >
            {tr("actions.edit", "Edit")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() =>
              manage(
                row.original.id,
                row.original.status === 1 ? "disable" : "enable",
                row.index
              )
            }
          >
            {row.original.status === 1
              ? tr("actions.disable", "Disable")
              : tr("actions.enable", "Enable")}
          </Button>
          <Button
            variant="destructive"
            size="sm"
            onClick={() => manage(row.original.id, "delete", row.index)}
          >
            {tr("actions.delete", "Delete")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() =>
              setOpenTopup({
                open: true,
                userId: row.original.id,
                username: row.original.username,
              })
            }
          >
            {tr("actions.topup", "Top Up")}
          </Button>
        </ResponsiveActionGroup>
      ),
    },
  ];

  const manage = async (
    id: number,
    action: "enable" | "disable" | "delete",
    idx: number
  ) => {
    try {
      let res: any;
      if (action === "delete") {
        // Unified API call - complete URL with /api prefix
        res = await api.delete(`/api/user/${id}`);
      } else {
        const body: any = { id, status: action === "enable" ? 1 : 2 };
        res = await api.put("/api/user/?status_only=true", body);
      }
      const { success } = res.data;
      if (success) {
        // Optimistic update like legacy
        const next = [...data];
        if (action === "delete") {
          next.splice(idx, 1);
        } else {
          next[idx].status = action === "enable" ? 1 : 2;
        }
        setData(next);
      }
    } catch (error) {
      const message =
        (error as any)?.response?.data?.message ||
        tr("notifications.action_failed_message", "Unable to apply change.");
      notify({
        type: "error",
        title: tr("notifications.action_failed_title", "Action failed"),
        message,
      });
    }
  };

  const toolbarActions = (
    <div
      className={cn(
        "flex gap-2",
        isMobile ? "flex-col w-full" : "items-center"
      )}
    >
      <Button
        onClick={() => navigate("/users/add")}
        className={cn(isMobile ? "w-full touch-target" : "")}
      >
        {tr("toolbar.add_user", "Add User")}
      </Button>
      <select
        className={cn(
          "h-11 sm:h-9 border rounded-md px-3 py-2 text-base sm:text-sm",
          isMobile ? "w-full" : ""
        )}
        value={sortBy}
        onChange={(e) => {
          setSortBy(e.target.value);
          setSortOrder("desc");
        }}
      >
        <option value="">{tr("toolbar.sort.default", "Default")}</option>
        <option value="quota">
          {tr("toolbar.sort.quota", "Remaining Quota")}
        </option>
        <option value="used_quota">
          {tr("toolbar.sort.used_quota", "Used Quota")}
        </option>
        <option value="username">
          {tr("toolbar.sort.username", "Username")}
        </option>
        <option value="id">{tr("toolbar.sort.id", "ID")}</option>
        <option value="created_at">
          {tr("toolbar.sort.register_time", "Register Time")}
        </option>
      </select>
      <Button
        variant="outline"
        size="sm"
        onClick={() => setSortOrder((o) => (o === "asc" ? "desc" : "asc"))}
        className={cn(isMobile ? "w-full touch-target" : "")}
      >
        {sortOrder === "asc"
          ? tr("toolbar.sort_order.asc", "ASC")
          : tr("toolbar.sort_order.desc", "DESC")}
      </Button>
    </div>
  );

  // Handlers for page change and page size change
  const handlePageChange = (newPageIndex: number, newPageSize: number) => {
    setSearchParams((prev) => {
      prev.set("p", (newPageIndex + 1).toString());
      return prev;
    });
    load(newPageIndex, newPageSize);
  };

  const handlePageSizeChange = (newPageSize: number) => {
    setPageSize(newPageSize);
    setPageIndex(0);
    // Don't call load here - let onPageChange handle it to avoid duplicate API calls
  };

  return (
    <ResponsivePageContainer
      title={tr("title", "Users")}
      description={tr("description", "Manage users")}
      actions={toolbarActions}
    >
      <Card>
        <CardContent className={cn(isMobile ? "p-4" : "p-6")}>
          <EnhancedDataTable
            columns={columns}
            data={data}
            floatingRowActions={(row) => (
              <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => navigate(`/users/edit/${row.id}`)}
                  title={tr("actions.edit", "Edit")}
                >
                  <Settings className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => {
                    const idx = data.findIndex((u) => u.id === row.id);
                    manage(
                      row.id,
                      row.status === 1 ? "disable" : "enable",
                      idx
                    );
                  }}
                  title={
                    row.status === 1
                      ? tr("actions.disable", "Disable")
                      : tr("actions.enable", "Enable")
                  }
                  className={
                    row.status === 1
                      ? "text-orange-600 hover:text-orange-700"
                      : "text-green-600 hover:text-green-700"
                  }
                >
                  {row.status === 1 ? (
                    <Ban className="h-4 w-4" />
                  ) : (
                    <CheckCircle className="h-4 w-4" />
                  )}
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() =>
                    setOpenTopup({
                      open: true,
                      userId: row.id,
                      username: row.username,
                    })
                  }
                  title={tr("actions.topup", "Top Up")}
                >
                  <CreditCard className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => {
                    const idx = data.findIndex((u) => u.id === row.id);
                    manage(row.id, "delete", idx);
                  }}
                  title={tr("actions.delete", "Delete")}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            )}
            pageIndex={pageIndex}
            pageSize={pageSize}
            total={total}
            onPageChange={handlePageChange}
            onPageSizeChange={handlePageSizeChange}
            sortBy={sortBy}
            sortOrder={sortOrder}
            onSortChange={(newSortBy, newSortOrder) => {
              setSortBy(newSortBy);
              setSortOrder(newSortOrder);
              // Let useEffect handle the reload to avoid double requests
            }}
            searchValue={searchKeyword}
            searchOptions={searchOptions}
            searchLoading={searchLoading}
            onSearchChange={searchUsers}
            onSearchValueChange={setSearchKeyword}
            onSearchSubmit={search}
            searchPlaceholder={tr(
              "search.placeholder",
              "Search users by username..."
            )}
            allowSearchAdditions={true}
            onRefresh={() => load(pageIndex, pageSize)}
            loading={loading}
            emptyMessage={tr(
              "empty",
              "No users found. Add your first user to get started."
            )}
            mobileCardLayout={true}
            hideColumnsOnMobile={[]}
            compactMode={isMobile}
          />
        </CardContent>
      </Card>

      {/* Create User Dialog */}
      <CreateUserDialog
        open={openCreate}
        onOpenChange={setOpenCreate}
        onCreated={() => load(pageIndex, pageSize)}
      />
      {/* Top Up Dialog */}
      <TopUpDialog
        open={openTopup.open}
        onOpenChange={(v) => setOpenTopup({ open: v })}
        userId={openTopup.userId}
        username={openTopup.username}
        onDone={() => load(pageIndex, pageSize)}
      />
    </ResponsivePageContainer>
  );
}

// Create User Dialog
function CreateUserDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onCreated: () => void;
}) {
  const schema = z.object({
    username: z.string().min(1),
    password: z.string().min(6),
    display_name: z.string().optional(),
  });
  type FormT = z.infer<typeof schema>;
  const form = useForm<FormT>({
    resolver: zodResolver(schema),
    defaultValues: { username: "", password: "", display_name: "" },
  });
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`users.dialogs.create.${key}`, { defaultValue, ...options }),
    [t]
  );
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{tr("title", "Create User")}</DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            className="space-y-3"
            onSubmit={form.handleSubmit(async (values) => {
              // Unified API call - complete URL with /api prefix
              const res = await api.post("/api/user/", {
                username: values.username,
                password: values.password,
                display_name: values.display_name || values.username,
              });
              if (res.data?.success) {
                onOpenChange(false);
                form.reset();
                onCreated();
              }
            })}
          >
            <FormField
              control={form.control}
              name="username"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {tr("fields.username.label", "Username")}
                  </FormLabel>
                  <FormControl>
                    <Input
                      placeholder={tr(
                        "fields.username.placeholder",
                        "Enter username"
                      )}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="password"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {tr("fields.password.label", "Password")}
                  </FormLabel>
                  <FormControl>
                    <Input
                      type="password"
                      placeholder={tr(
                        "fields.password.placeholder",
                        "Enter password"
                      )}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="display_name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {tr("fields.display_name.label", "Display Name")}
                  </FormLabel>
                  <FormControl>
                    <Input
                      placeholder={tr(
                        "fields.display_name.placeholder",
                        "Enter display name"
                      )}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="pt-2 flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                {tr("actions.close", "Close")}
              </Button>
              <Button type="submit">{tr("actions.create", "Create")}</Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

// Top Up Dialog
function TopUpDialog({
  open,
  onOpenChange,
  userId,
  username,
  onDone,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  userId?: number;
  username?: string;
  onDone: () => void;
}) {
  const schema = z.object({
    quota: z.coerce.number().int(),
    remark: z.string().optional(),
  });
  type FormT = z.infer<typeof schema>;
  const form = useForm<FormT>({
    resolver: zodResolver(schema),
    defaultValues: { quota: 0, remark: "" },
  });
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`users.dialogs.topup.${key}`, { defaultValue, ...options }),
    [t]
  );
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {tr("title", "Top Up {{username}}", {
              username: username ? `@${username}` : "",
            })}
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            className="space-y-3"
            onSubmit={form.handleSubmit(async (values) => {
              if (!userId) return;
              // Unified API call - complete URL with /api prefix
              const res = await api.post("/api/topup", {
                user_id: userId,
                quota: values.quota,
                remark: values.remark,
              });
              if (res.data?.success) {
                onOpenChange(false);
                form.reset();
                onDone();
              }
            })}
          >
            <FormField
              control={form.control}
              name="quota"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{tr("fields.quota.label", "Quota")}</FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      placeholder={tr(
                        "fields.quota.placeholder",
                        "Enter quota change"
                      )}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="remark"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{tr("fields.remark.label", "Remark")}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={tr("fields.remark.placeholder", "Optional")}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="pt-2 flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                {tr("actions.close", "Close")}
              </Button>
              <Button type="submit">{tr("actions.submit", "Submit")}</Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
