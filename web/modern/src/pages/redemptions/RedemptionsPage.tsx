import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { DataTable } from "@/components/ui/data-table";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { ResponsiveActionGroup } from "@/components/ui/responsive-action-group";
import { ResponsivePageContainer } from "@/components/ui/responsive-container";
import { SearchableDropdown } from "@/components/ui/searchable-dropdown";
import { TimestampDisplay } from "@/components/ui/timestamp";
import { useResponsive } from "@/hooks/useResponsive";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import { zodResolver } from "@hookform/resolvers/zod";
import type { ColumnDef } from "@tanstack/react-table";
import { useCallback, useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { useNavigate, useSearchParams } from "react-router-dom";
import * as z from "zod";

interface RedemptionRow {
  id: number;
  name: string;
  key: string;
  status: number;
  created_time: number;
  quota: number;
}

export function RedemptionsPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { isMobile } = useResponsive();
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`redemptions.${key}`, { defaultValue, ...options }),
    [t]
  );

  const renderStatus = useCallback(
    (status: number) => {
      const map: Record<number, { text: string; cls: string }> = {
        1: { text: tr("status.unused", "Unused"), cls: "text-green-600" },
        2: { text: tr("status.disabled", "Disabled"), cls: "text-red-600" },
        3: { text: tr("status.used", "Used"), cls: "text-gray-600" },
      };
      const v = map[status] || {
        text: tr("status.unknown", "Unknown"),
        cls: "text-muted-foreground",
      };
      return <span className={`text-sm ${v.cls}`}>{v.text}</span>;
    },
    [tr]
  );

  const [data, setData] = useState<RedemptionRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [pageIndex, setPageIndex] = useState(
    Math.max(0, parseInt(searchParams.get("p") || "1") - 1)
  );
  const [pageSize, setPageSize] = useState(10);
  const [total, setTotal] = useState(0);
  const [searchKeyword, setSearchKeyword] = useState("");
  const [sortBy, setSortBy] = useState("");
  const [sortOrder, setSortOrder] = useState<"asc" | "desc">("desc");
  const [open, setOpen] = useState(false);
  const [generatedKeys, setGeneratedKeys] = useState<string[] | null>(null);
  const mounted = useRef(false);

  const schema = z.object({
    name: z
      .string()
      .min(1, tr("edit.validation.name_required", "Name is required"))
      .max(20, tr("edit.validation.name_max", "Max 20 chars")),
    count: z.coerce
      .number()
      .int()
      .min(1, tr("edit.validation.count_min", "Count must be positive"))
      .max(100, tr("edit.validation.count_max", "Count cannot exceed 100")),
    quota: z.coerce
      .number()
      .int()
      .min(0, tr("edit.validation.quota_min", "Quota cannot be negative")),
  });
  type CreateForm = z.infer<typeof schema>;
  const form = useForm<CreateForm>({
    resolver: zodResolver(schema),
    defaultValues: { name: "", count: 1, quota: 0 },
  });

  const load = async (p = 0, size = pageSize) => {
    setLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      let url = `/api/redemption/?p=${p}&size=${size}`;
      if (sortBy) url += `&sort=${sortBy}&order=${sortOrder}`;
      const res = await api.get(url);
      const { success, data, total } = res.data;
      if (success) {
        setData(data);
        setTotal(total);
        setPageIndex(p);
      }
    } finally {
      setLoading(false);
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
      load(0);
    }
  }, [sortBy, sortOrder]);

  const search = async () => {
    if (!searchKeyword.trim()) return load(0, pageSize);
    setLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      let url = `/api/redemption/search?keyword=${encodeURIComponent(searchKeyword)}`;
      if (sortBy) url += `&sort=${sortBy}&order=${sortOrder}`;
      url += `&size=${pageSize}`;
      const res = await api.get(url);
      const { success, data } = res.data;
      if (success) {
        setData(data);
        setPageIndex(0);
      }
    } finally {
      setLoading(false);
    }
  };

  const columns: ColumnDef<RedemptionRow>[] = [
    { header: tr("columns.id", "ID"), accessorKey: "id" },
    { header: tr("columns.name", "Name"), accessorKey: "name" },
    { header: tr("columns.code", "Code"), accessorKey: "key" },
    {
      header: tr("columns.quota", "Quota"),
      accessorKey: "quota",
      cell: ({ row }) => (
        <span
          className="font-mono text-sm"
          title={`${tr("columns.quota", "Quota")}: ${row.original.quota ? `$${(row.original.quota / 500000).toFixed(2)}` : "$0.00"}`}
        >
          {row.original.quota
            ? `$${(row.original.quota / 500000).toFixed(2)}`
            : "$0.00"}
        </span>
      ),
    },
    {
      header: tr("columns.status", "Status"),
      cell: ({ row }) => renderStatus(row.original.status),
    },
    {
      header: tr("columns.created", "Created"),
      cell: ({ row }) => (
        <TimestampDisplay
          timestamp={row.original.created_time}
          className="text-sm"
        />
      ),
    },
    {
      header: tr("columns.actions", "Actions"),
      cell: ({ row }) => (
        <ResponsiveActionGroup justify="start">
          <Button
            variant="outline"
            size="sm"
            onClick={() => navigate(`/redemptions/edit/${row.original.id}`)}
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
        </ResponsiveActionGroup>
      ),
    },
  ];

  const manage = async (
    id: number,
    action: "enable" | "disable" | "delete",
    idx: number
  ) => {
    let res: any;
    if (action === "delete") {
      // Unified API call - complete URL with /api prefix
      res = await api.delete(`/api/redemption/${id}`);
    } else {
      const body: any = { id, status: action === "enable" ? 1 : 2 };
      res = await api.put("/api/redemption/?status_only=true", body);
    }
    if (res.data?.success) {
      const next = [...data];
      if (action === "delete") next.splice(idx, 1);
      else next[idx].status = action === "enable" ? 1 : 2;
      setData(next);
    }
  };

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
      title={tr("title", "Redemptions")}
      description={tr("description", "Manage recharge codes")}
      actions={
        <div
          className={cn(
            "flex gap-2 flex-wrap",
            isMobile ? "w-full" : "items-center"
          )}
        >
          <Button
            onClick={() => navigate("/redemptions/add")}
            className={cn(isMobile ? "w-full touch-target" : "")}
          >
            {tr("actions.add", "Add Redemption")}
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
            <option value="">{tr("toolbar.default", "Default")}</option>
            <option value="id">{tr("toolbar.id", "ID")}</option>
            <option value="name">{tr("toolbar.name", "Name")}</option>
            <option value="status">{tr("toolbar.status", "Status")}</option>
            <option value="quota">{tr("toolbar.quota", "Quota")}</option>
            <option value="created_time">
              {tr("toolbar.created_time", "Created Time")}
            </option>
            <option value="redeemed_time">
              {tr("toolbar.redeemed_time", "Redeemed Time")}
            </option>
          </select>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setSortOrder((o) => (o === "asc" ? "desc" : "asc"))}
            className={cn(isMobile ? "w-full touch-target" : "")}
          >
            {sortOrder.toUpperCase()}
          </Button>
          <Button
            onClick={() => load(pageIndex, pageSize)}
            disabled={loading}
            variant="outline"
            className={cn(isMobile ? "w-full touch-target" : "")}
          >
            {tr("actions.refresh", "Refresh")}
          </Button>
        </div>
      }
    >
      <Card>
        <CardContent className={cn(isMobile ? "p-4" : "p-6")}>
          <div
            className={cn(
              "flex gap-2 mb-3 flex-wrap",
              isMobile ? "w-full" : "items-center"
            )}
          >
            <SearchableDropdown
              value={searchKeyword}
              placeholder={tr(
                "search.placeholder",
                "Search redemptions by name..."
              )}
              searchPlaceholder={tr(
                "search.dropdown_placeholder",
                "Type redemption name..."
              )}
              options={[]}
              searchEndpoint="/api/redemption/search"
              transformResponse={(data) =>
                Array.isArray(data)
                  ? data.map((r: any) => ({
                      key: String(r.id),
                      value: r.name,
                      text: r.name,
                    }))
                  : []
              }
              onChange={(value) => setSearchKeyword(value)}
              clearable
              className={cn(isMobile ? "w-full" : "max-w-md")}
            />
            <Button
              onClick={search}
              disabled={loading}
              className={cn(isMobile ? "w-full touch-target" : "")}
            >
              {tr("actions.search", "Search")}
            </Button>
          </div>
          <DataTable
            columns={columns}
            data={data}
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
            }}
            loading={loading}
          />
        </CardContent>
      </Card>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {tr("dialog.title", "Generate Redemption Codes")}
            </DialogTitle>
          </DialogHeader>
          <Form {...form}>
            <form
              className="space-y-3"
              onSubmit={form.handleSubmit(async (values) => {
                // Unified API call - complete URL with /api prefix
                const res = await api.post("/api/redemption/", {
                  name: values.name,
                  count: values.count,
                  quota: values.quota,
                });
                if (res.data?.success) {
                  setGeneratedKeys(res.data.data || []);
                  load(pageIndex, pageSize);
                }
              })}
            >
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{tr("fields.name.label", "Name")}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={tr(
                          "fields.name.placeholder",
                          "Enter redemption name"
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
                name="count"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {tr("fields.count.label", "Count (1-100)")}
                    </FormLabel>
                    <FormControl>
                      <Input type="number" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="quota"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{tr("fields.quota.label", "Quota")}</FormLabel>
                    <FormControl>
                      <Input type="number" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <div className="pt-2 flex justify-end gap-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setOpen(false)}
                >
                  {tr("actions.close", "Close")}
                </Button>
                <Button type="submit">
                  {tr("actions.generate", "Generate")}
                </Button>
              </div>

              {generatedKeys && (
                <div className="mt-4">
                  <div className="text-sm mb-2">
                    {tr("dialog.generated_codes", "Generated Codes:")}
                  </div>
                  <textarea
                    className="w-full h-32 p-2 border rounded"
                    readOnly
                    value={generatedKeys.join("\n")}
                  />
                </div>
              )}
            </form>
          </Form>
        </DialogContent>
      </Dialog>
    </ResponsivePageContainer>
  );
}
