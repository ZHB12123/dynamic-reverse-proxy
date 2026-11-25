(function () {
  "use strict";

  const $ = (selector, root) => (root || document).querySelector(selector);

  const el = {
    connState: $("#connState"),
    refreshBtn: $("#refreshBtn"),
    portForm: $("#portForm"),
    portInput: $("#portInput"),
    filePathInput: $("#filePathInput"),
    shareStatic: $("#shareStatic"),
    filePathField: $("#filePathField"),
    routeList: $("#routeList"),
    routeSummary: $("#routeSummary"),
    emptyState: $("#emptyState"),
    routeModal: $("#routeModal"),
    routeModalTitle: $("#routeModalTitle"),
    routeModalSubtitle: $("#routeModalSubtitle"),
    routeForm: $("#routeForm"),
    locationInput: $("#locationInput"),
    proxyPassInput: $("#proxyPassInput"),
    regexInput: $("#regexInput"),
    useSystemProxyInput: $("#useSystemProxyInput"),
    luaRequestInput: $("#luaRequestInput"),
    luaResponseInput: $("#luaResponseInput"),
    routeSubmit: $("#routeSubmit"),
    toast: $("#toast"),
  };

  let routes = [];
  let expandedPort = null;
  let editing = null;
  let modalPort = null;
  let toastTimer = null;

  function escapeHtml(value) {
    return String(value == null ? "" : value).replace(/[&<>"']/g, function (ch) {
      return {
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&#39;",
      }[ch];
    });
  }

  function toast(message, type) {
    el.toast.textContent = message;
    el.toast.className = "toast show" + (type ? " " + type : "");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () {
      el.toast.className = "toast";
    }, 3200);
  }

  function setConn(ok) {
    el.connState.className = "pill " + (ok ? "pill-ok" : "pill-err");
    el.connState.textContent = ok ? "已连接" : "连接失败";
  }

  function setBusy(button, busy, label) {
    button.disabled = busy;
    button.textContent = label;
  }

  function syncShareStatic() {
    const share = el.shareStatic.checked;
    el.filePathInput.disabled = !share;
    el.filePathField.classList.toggle("is-disabled", !share);
  }

  async function api(path, payload) {
    const response = await fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload || {}),
    });
    const text = await response.text();
    if (!response.ok) {
      throw new Error(text || "HTTP " + response.status);
    }
    return text;
  }

  async function mutate(path, payload) {
    const text = await api(path, payload);
    const failed = !/success/i.test(text) && /(error|invalid|failed|unexpected|cannot|refused)/i.test(text);
    toast(text || "操作完成", failed ? "err" : "ok");
    await loadRoutes({ silent: true });
  }

  async function loadRoutes(options) {
    const silent = Boolean(options && options.silent);
    try {
      const response = await fetch("/query_route", { method: "POST" });
      const text = await response.text();
      if (!response.ok) {
        throw new Error(text || "HTTP " + response.status);
      }
      let data = [];
      if (text.trim()) {
        data = JSON.parse(text);
      }
      routes = Array.isArray(data) ? data : [];
      setConn(true);
      render();
      if (!silent) {
        toast("配置已刷新", "ok");
      }
    } catch (err) {
      setConn(false);
      toast("加载失败：" + err.message, "err");
    }
  }

  function routeExists(port, location) {
    const portData = routes.find(function (item) {
      return item.port === port;
    });
    return Boolean(
      portData &&
        (portData.mappings || []).some(function (mapping) {
          return mapping.location === location;
        })
    );
  }

  function findMapping(port, location) {
    const portData = routes.find(function (item) {
      return item.port === port;
    });
    return portData && (portData.mappings || []).find(function (mapping) {
      return mapping.location === location;
    });
  }

  function render() {
    const totalRoutes = routes.reduce(function (sum, port) {
      return sum + ((port.mappings && port.mappings.length) || 0);
    }, 0);

    el.routeSummary.textContent = routes.length + " 个端口 · " + totalRoutes + " 条路由";

    if (expandedPort && !routes.some(function (item) { return item.port === expandedPort; })) {
      expandedPort = null;
    }
    if (editing && !routeExists(editing.port, editing.location)) {
      if (modalPort) {
        closeRouteModal();
      } else {
        editing = null;
      }
    }

    renderPortList();
  }

  function renderPortList() {
    if (!routes.length) {
      el.routeList.innerHTML = "";
      el.emptyState.classList.remove("hidden");
      return;
    }

    el.emptyState.classList.add("hidden");
    el.routeList.innerHTML = routes.map(renderPortCard).join("");
  }

  function renderPortCard(port) {
    const isOpen = expandedPort === port.port;
    const count = (port.mappings && port.mappings.length) || 0;
    const portAttr = escapeHtml(port.port);
    const staticLabel = port.file_path
      ? escapeHtml(port.file_path)
      : "不分享静态文件目录";

    return (
      '<article class="port-card' + (isOpen ? " open" : "") + '" data-port="' + portAttr + '">' +
      '<header class="port-head">' +
      '<button type="button" class="port-toggle" data-action="toggle-port" data-port="' + portAttr +
      '" aria-expanded="' + (isOpen ? "true" : "false") + '">' +
      '<span class="chevron" aria-hidden="true">›</span>' +
      "<code>:" + portAttr + "</code>" +
      '<span class="file-path">' + staticLabel + "</span>" +
      '<span class="count-badge">' + count + " 条路由</span>" +
      "</button>" +
      '<button type="button" class="btn btn-danger-ghost btn-sm" data-action="drop-port" data-port="' +
      portAttr + '">删除端口</button>' +
      "</header>" +
      (isOpen ? renderPortBody(port) : "") +
      "</article>"
    );
  }

  function renderPortBody(port) {
    const mappings = port.mappings || [];
    const noRouteText = port.file_path
      ? "该端口暂无路由，所有请求将由静态目录响应。"
      : "该端口暂无路由，且未分享静态目录，请求会返回 404。";

    const rows = mappings.length
      ? mappings
          .map(function (mapping) {
            const portAttr = escapeHtml(port.port);
            const locationAttr = escapeHtml(mapping.location);
            return (
              "<tr>" +
              '<td><code class="path">' + escapeHtml(mapping.location) + "</code></td>" +
              '<td><span class="upstream">' + escapeHtml(mapping.proxy_pass) + "</span></td>" +
              "<td>" + routeBadges(mapping) + "</td>" +
              '<td class="row-actions">' +
              '<button type="button" class="btn btn-ghost btn-sm" data-action="edit" data-port="' +
              portAttr + '" data-location="' + locationAttr + '">修改</button>' +
              '<button type="button" class="btn btn-danger-ghost btn-sm" data-action="drop-route" data-port="' +
              portAttr + '" data-location="' + locationAttr + '">删除</button>' +
              "</td>" +
              "</tr>"
            );
          })
          .join("")
      : '<tr><td colspan="4" class="no-route">' + noRouteText + "</td></tr>";

    return (
      '<div class="port-body">' +
      '<div class="port-toolbar">' +
      '<span class="muted">路由列表</span>' +
      '<button type="button" class="btn btn-primary btn-sm" data-action="add-route" data-port="' +
      escapeHtml(port.port) + '">+ 添加路由</button>' +
      "</div>" +
      '<div class="table-wrap"><table>' +
      "<thead><tr><th>匹配路径</th><th>上游地址</th><th>标记</th><th></th></tr></thead>" +
      "<tbody>" + rows + "</tbody>" +
      "</table></div>" +
      "</div>"
    );
  }

  function routeBadges(mapping) {
    const badges = [];
    if (mapping.regex) {
      badges.push('<span class="badge badge-regex">正则</span>');
    }
    if (mapping.use_system_proxy === false) {
      badges.push('<span class="badge badge-direct">直连</span>');
    }
    if (mapping.lua_script_request) {
      badges.push('<span class="badge badge-lua">请求脚本</span>');
    }
    if (mapping.lua_script_response) {
      badges.push('<span class="badge badge-lua">响应脚本</span>');
    }
    if (!badges.length) {
      return '<span class="muted">—</span>';
    }
    return '<div class="script-badges">' + badges.join("") + "</div>";
  }

  function openRouteModal(port, mapping) {
    modalPort = port;
    editing = mapping ? { port: port, location: mapping.location } : null;

    el.routeForm.reset();
    el.routeModalTitle.textContent = mapping ? "编辑转发路由" : "添加转发路由";
    el.routeModalSubtitle.textContent = "端口 :" + port;
    el.routeSubmit.textContent = mapping ? "保存修改" : "添加路由";

    if (mapping) {
      el.locationInput.value = mapping.location;
      el.proxyPassInput.value = mapping.proxy_pass || "";
      el.regexInput.checked = Boolean(mapping.regex);
      el.useSystemProxyInput.checked = mapping.use_system_proxy !== false;
      el.luaRequestInput.value = mapping.lua_script_request || "";
      el.luaResponseInput.value = mapping.lua_script_response || "";
    }

    el.routeModal.classList.remove("hidden");
    document.body.classList.add("modal-open");
    el.locationInput.focus();
  }

  function closeRouteModal() {
    el.routeModal.classList.add("hidden");
    document.body.classList.remove("modal-open");
    modalPort = null;
    editing = null;
    el.routeForm.reset();
    el.routeSubmit.textContent = "添加路由";
  }

  function isHttpUrl(value) {
    try {
      const parsed = new URL(value);
      return parsed.protocol === "http:" || parsed.protocol === "https:";
    } catch (err) {
      return false;
    }
  }

  el.portForm.addEventListener("submit", async function (event) {
    event.preventDefault();
    const port = el.portInput.value.trim();
    const shareStatic = el.shareStatic.checked;
    const filePath = shareStatic ? el.filePathInput.value.trim() : "";
    const portNumber = Number(port);

    if (!/^\d+$/.test(port) || portNumber < 1 || portNumber > 65535) {
      toast("请输入 1-65535 之间的端口号", "err");
      return;
    }
    if (shareStatic && !filePath) {
      toast("请填写静态文件目录，或关闭“分享静态文件目录”", "err");
      return;
    }
    if (routes.some(function (item) { return item.port === port; })) {
      if (!window.confirm("端口 :" + port + " 已存在，重复添加不会生效。是否继续？")) {
        return;
      }
    }

    const button = el.portForm.querySelector('button[type="submit"]');
    setBusy(button, true, "添加中…");
    try {
      await mutate("/add_port", { port: port, filePath: filePath });
      el.portInput.value = "";
    } catch (err) {
      toast("添加失败：" + err.message, "err");
    } finally {
      setBusy(button, false, "添加端口");
    }
  });

  el.routeList.addEventListener("click", async function (event) {
    const button = event.target.closest("button[data-action]");
    if (!button) {
      return;
    }

    const action = button.dataset.action;
    const port = button.dataset.port;

    if (action === "toggle-port") {
      if (expandedPort === port) {
        expandedPort = null;
        if (editing && editing.port === port) {
          editing = null;
        }
      } else {
        expandedPort = port;
        if (editing && editing.port !== port) {
          editing = null;
        }
      }
      renderPortList();
      return;
    }

    if (action === "add-route") {
      openRouteModal(port, null);
      return;
    }

    if (action === "edit") {
      const mapping = findMapping(port, button.dataset.location);
      if (!mapping) {
        toast("未找到该路由，请刷新后重试", "err");
        return;
      }
      openRouteModal(port, mapping);
      return;
    }

    if (action === "drop-route") {
      const location = button.dataset.location;
      if (!window.confirm("确定删除端口 :" + port + " 的路径 " + location + " 吗？")) {
        return;
      }
      button.disabled = true;
      try {
        await mutate("/drop_route", { port: port, location: location });
      } catch (err) {
        toast("删除失败：" + err.message, "err");
        button.disabled = false;
      }
      return;
    }

    if (action === "drop-port") {
      if (!window.confirm("确定删除端口 :" + port + " 及其全部路由吗？该端口将停止监听。")) {
        return;
      }
      if (editing && editing.port === port) {
        editing = null;
      }
      if (expandedPort === port) {
        expandedPort = null;
      }
      button.disabled = true;
      try {
        await mutate("/drop_port", { port: port });
      } catch (err) {
        toast("删除失败：" + err.message, "err");
        button.disabled = false;
      }
    }
  });

  el.routeForm.addEventListener("submit", async function (event) {
    event.preventDefault();
    const port = modalPort;
    if (!port) {
      return;
    }

    const location = el.locationInput.value.trim();
    const proxyPass = el.proxyPassInput.value.trim();
    const useRegex = el.regexInput.checked;

    if (!location) {
      toast("匹配路径不能为空", "err");
      return;
    }
    if (!useRegex && !location.startsWith("/")) {
      toast("精确匹配路径必须以 / 开头", "err");
      return;
    }
    if (!isHttpUrl(proxyPass)) {
      toast("上游地址必须是 http:// 或 https:// 开头的合法 URL", "err");
      return;
    }

    const wasEditing = Boolean(editing && editing.port === port);
    const oldLocation = wasEditing ? editing.location : null;

    const overwriting = routeExists(port, location) && !(wasEditing && oldLocation === location);
    if (overwriting && !window.confirm("端口 :" + port + " 已存在路径 " + location + " 的路由，保存后将覆盖。是否继续？")) {
      return;
    }

    setBusy(el.routeSubmit, true, "保存中…");
    try {
      if (wasEditing && oldLocation !== location) {
        await api("/drop_route", { port: port, location: oldLocation });
      }
      await mutate("/add_route", {
        port: port,
        location: location,
        proxy_pass: proxyPass,
        regex: useRegex,
        use_system_proxy: el.useSystemProxyInput.checked,
        lua_script_request: el.luaRequestInput.value,
        lua_script_response: el.luaResponseInput.value,
      });
      closeRouteModal();
    } catch (err) {
      toast("保存失败：" + err.message, "err");
    } finally {
      setBusy(el.routeSubmit, false, editing ? "保存修改" : "添加路由");
    }
  });

  el.routeModal.addEventListener("click", function (event) {
    if (event.target.closest('[data-action="close-route-modal"]')) {
      closeRouteModal();
    }
  });

  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape" && !el.routeModal.classList.contains("hidden")) {
      closeRouteModal();
    }
  });

  el.shareStatic.addEventListener("change", syncShareStatic);

  el.refreshBtn.addEventListener("click", function () {
    editing = null;
    loadRoutes();
  });

  syncShareStatic();
  loadRoutes({ silent: true });
})();
