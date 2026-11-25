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
    toast: $("#toast"),
  };

  let routes = [];
  let expandedPort = null;
  let editing = null;
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

  function getRouteForm(port) {
    const forms = el.routeList.querySelectorAll("form[data-route-form]");
    for (let index = 0; index < forms.length; index += 1) {
      if (forms[index].dataset.port === port) {
        return forms[index];
      }
    }
    return null;
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
      editing = null;
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
              "<td>" + scriptBadges(mapping) + "</td>" +
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
      '<div class="table-wrap"><table>' +
      "<thead><tr><th>匹配路径</th><th>上游地址</th><th>Lua 脚本</th><th></th></tr></thead>" +
      "<tbody>" + rows + "</tbody>" +
      "</table></div>" +
      renderRouteForm(port) +
      "</div>"
    );
  }

  function scriptBadges(mapping) {
    const badges = [];
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

  function renderRouteForm(port) {
    return (
      '<form class="route-form" data-route-form data-port="' + escapeHtml(port.port) + '" autocomplete="off">' +
      '<div class="route-form-head">' +
      "<div>" +
      '<h3 data-role="form-title">添加转发路由</h3>' +
      "<p>按请求路径精确匹配，命中后转发到上游服务。</p>" +
      "</div>" +
      '<span class="badge badge-edit hidden" data-role="edit-badge">编辑中</span>' +
      "</div>" +
      '<div class="route-fields">' +
      '<label class="field">' +
      "<span>匹配路径 <em>*</em></span>" +
      '<input name="location" type="text" placeholder="/api" required>' +
      "<small>精确匹配且区分大小写，例如 /api 不会匹配 /api/users。</small>" +
      "</label>" +
      '<label class="field">' +
      "<span>上游地址 <em>*</em></span>" +
      '<input name="proxyPass" type="text" placeholder="http://127.0.0.1:9000" required>' +
      "<small>转发时使用该 URL 的路径，原始查询参数会原样保留。</small>" +
      "</label>" +
      "</div>" +
      '<div class="route-fields">' +
      '<label class="field">' +
      "<span>请求脚本（Lua，可选）</span>" +
      '<textarea name="luaRequest" rows="4" spellcheck="false" placeholder=\'headers["X-Proxy"] = "dynamic-reverse-proxy"\'></textarea>' +
      "</label>" +
      '<label class="field">' +
      "<span>响应脚本（Lua，可选）</span>" +
      '<textarea name="luaResponse" rows="4" spellcheck="false" placeholder=\'body = string.gsub(body, "foo", "bar")\'></textarea>' +
      "</label>" +
      "</div>" +
      '<p class="form-tip">脚本可读写全局 <code>headers</code> 表，并给全局 <code>body</code> 赋值以替换消息体。</p>' +
      '<div class="form-actions">' +
      '<button type="submit" class="btn btn-primary" data-role="submit">添加路由</button>' +
      '<button type="button" class="btn btn-ghost hidden" data-action="cancel-edit" data-role="cancel">取消编辑</button>' +
      "</div>" +
      "</form>"
    );
  }

  function setFormMode(form, isEdit) {
    form.querySelector('[data-role="form-title"]').textContent = isEdit ? "编辑转发路由" : "添加转发路由";
    form.querySelector('[data-role="submit"]').textContent = isEdit ? "保存修改" : "添加路由";
    form.querySelector('[data-role="edit-badge"]').classList.toggle("hidden", !isEdit);
    form.querySelector('[data-role="cancel"]').classList.toggle("hidden", !isEdit);
  }

  function resetRouteForm(form) {
    form.reset();
    editing = null;
    setFormMode(form, false);
  }

  function startEdit(port, location) {
    const mapping = findMapping(port, location);
    if (!mapping) {
      toast("未找到该路由，请刷新后重试", "err");
      return;
    }

    if (expandedPort !== port) {
      expandedPort = port;
      editing = null;
      renderPortList();
    }

    const form = getRouteForm(port);
    if (!form) {
      toast("无法打开路由编辑表单，请刷新后重试", "err");
      return;
    }

    editing = { port: port, location: location };
    form.elements.location.value = mapping.location;
    form.elements.proxyPass.value = mapping.proxy_pass || "";
    form.elements.luaRequest.value = mapping.lua_script_request || "";
    form.elements.luaResponse.value = mapping.lua_script_response || "";
    setFormMode(form, true);
    form.scrollIntoView({ behavior: "smooth", block: "nearest" });
    form.elements.location.focus({ preventScroll: true });
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

    if (action === "edit") {
      startEdit(port, button.dataset.location);
      return;
    }

    if (action === "cancel-edit") {
      const form = button.closest("form[data-route-form]");
      if (form) {
        resetRouteForm(form);
      }
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

  el.routeList.addEventListener("submit", async function (event) {
    const form = event.target.closest("form[data-route-form]");
    if (!form) {
      return;
    }
    event.preventDefault();

    const port = form.dataset.port;
    const location = form.elements.location.value.trim();
    const proxyPass = form.elements.proxyPass.value.trim();

    if (!location.startsWith("/")) {
      toast("匹配路径必须以 / 开头", "err");
      return;
    }
    if (!isHttpUrl(proxyPass)) {
      toast("上游地址必须是 http:// 或 https:// 开头的合法 URL", "err");
      return;
    }

    const wasEditing = Boolean(editing && editing.port === port);
    const oldLocation = wasEditing ? editing.location : null;
    const submit = form.querySelector('[data-role="submit"]');

    const overwriting = routeExists(port, location) && !(wasEditing && oldLocation === location);
    if (overwriting && !window.confirm("端口 :" + port + " 已存在路径 " + location + " 的路由，保存后将覆盖。是否继续？")) {
      return;
    }

    setBusy(submit, true, "保存中…");
    try {
      if (wasEditing && oldLocation !== location) {
        await api("/drop_route", { port: port, location: oldLocation });
      }
      await mutate("/add_route", {
        port: port,
        location: location,
        proxy_pass: proxyPass,
        lua_script_request: form.elements.luaRequest.value,
        lua_script_response: form.elements.luaResponse.value,
      });
      editing = null;
    } catch (err) {
      toast("保存失败：" + err.message, "err");
    } finally {
      if (form.isConnected) {
        setBusy(submit, false, editing && editing.port === port ? "保存修改" : "添加路由");
      }
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
