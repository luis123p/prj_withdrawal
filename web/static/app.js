// Cajero Web - interfaz de prj_withdrawal (consume la API Go en /api/*)
// Autor: Paucar Ventura, Luis Antonio
"use strict";

const $ = (s) => document.querySelector(s);
const $$ = (s) => document.querySelectorAll(s);
const fmt = (n) => "S/ " + Number(n).toLocaleString("es-PE", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

let cuentas = [];
let seleccionada = null;
let tipoPulsado = "retiro";

async function api(ruta, opciones = {}) {
  const r = await fetch(ruta, {
    headers: { "Content-Type": "application/json" },
    ...opciones,
  });
  const datos = await r.json().catch(() => ({}));
  return { ok: r.ok, status: r.status, datos };
}

function esc(t) {
  return String(t).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
}

// ---------------- cuentas ----------------
async function cargarCuentas() {
  const { datos } = await api("/api/cuentas");
  cuentas = datos;
  if (!seleccionada && cuentas.length) seleccionada = cuentas[0].titular;
  pintarCuentas();
  pintarFiltro();
}

function pintarCuentas() {
  const ul = $("#lista-cuentas");
  ul.innerHTML = cuentas
    .map((c) => `<li data-t="${esc(c.titular)}" class="${c.titular === seleccionada ? "sel" : ""}">
                   <span>${esc(c.titular)}</span><span>${fmt(c.saldo)}</span></li>`)
    .join("");
  ul.querySelectorAll("li").forEach((li) =>
    li.addEventListener("click", () => {
      seleccionada = li.dataset.t;
      $("#resultado").className = "resultado oculto";
      pintarCuentas();
      cargarRecientes();
    })
  );
  const c = cuentas.find((x) => x.titular === seleccionada);
  $("#titular-sel").textContent = c ? c.titular : "-";
  $("#saldo-sel").textContent = fmt(c ? c.saldo : 0);
}

function pintarFiltro() {
  const sel = $("#filtro-titular");
  const actual = sel.value;
  sel.innerHTML = '<option value="">Todos los titulares</option>' +
    cuentas.map((c) => `<option>${esc(c.titular)}</option>`).join("");
  sel.value = actual;
}

$("#form-cuenta").addEventListener("submit", async (e) => {
  e.preventDefault();
  const titular = $("#nc-titular").value.trim();
  const saldo = parseFloat($("#nc-saldo").value);
  const { ok, datos } = await api("/api/cuentas", { method: "POST", body: JSON.stringify({ titular, saldo }) });
  const msg = $("#nc-msg");
  if (!ok) {
    msg.style.color = "var(--rojo)";
    msg.textContent = datos.error || "No se pudo crear";
    return;
  }
  msg.style.color = "var(--verde)";
  msg.textContent = "Cuenta creada";
  seleccionada = titular;
  $("#resultado").className = "resultado oculto";
  e.target.reset();
  await cargarCuentas();
  cargarRecientes();
});

// ---------------- operaciones ----------------
$$(".rapidos button").forEach((b) => b.addEventListener("click", () => { $("#monto").value = b.dataset.m; $("#monto").focus(); }));
$$(".botones .btn").forEach((b) => b.addEventListener("click", () => { tipoPulsado = b.dataset.tipo; }));

$("#form-op").addEventListener("submit", async (e) => {
  e.preventDefault();
  if (!seleccionada) return;
  const monto = parseFloat($("#monto").value);
  if (Number.isNaN(monto)) return;
  $$(".botones .btn").forEach((b) => (b.disabled = true));
  const { ok, status, datos } = await api("/api/operaciones", {
    method: "POST",
    body: JSON.stringify({ titular: seleccionada, tipo: tipoPulsado, monto }),
  });
  $$(".botones .btn").forEach((b) => (b.disabled = false));
  mostrarResultado(ok, status, datos);
  if (datos.transaccion && datos.transaccion.estado === "EXITOSA") $("#monto").value = "";
  await cargarCuentas();
  cargarRecientes();
});

function mostrarResultado(ok, status, d) {
  const div = $("#resultado");
  let clase, titulo, texto;
  if (!ok && !d.transaccion) {
    clase = "ERROR"; titulo = "Error " + status; texto = d.error || "";
  } else if (status === 500) {
    clase = "ERROR"; titulo = "Operación revertida"; texto = d.error;
  } else {
    const t = d.transaccion;
    clase = t.estado;
    const op = t.tipo === "RETIRO" ? "Retiro" : "Abono";
    titulo = { EXITOSA: `${op} exitoso`, RECHAZADA: `${op} rechazado`, ERROR: "Monto inválido" }[t.estado];
    texto = t.estado === "EXITOSA"
      ? `${op} de ${fmt(t.monto)} realizado. Nuevo saldo: ${fmt(t.saldo_final)}`
      : `${t.mensaje}. El saldo no cambió (${fmt(t.saldo_final)}).`;
    texto += ` <span style="opacity:.7">· ${t.id}</span>`;
  }
  div.className = "resultado " + clase;
  div.innerHTML = `<b>${esc(titulo)}</b>${texto}`;
}

// ---------------- tablas ----------------
function filas(lista, conTitular) {
  if (!lista.length) return `<tr><td class="vacio" colspan="9">Sin movimientos todavía</td></tr>`;
  return lista.map((t) => `<tr>
      <td>${t.id}</td><td>${t.fecha}</td>${conTitular ? `<td>${esc(t.titular)}</td>` : ""}
      <td class="tipo-${t.tipo}">${t.tipo}</td>
      <td class="num">${fmt(t.monto)}</td><td class="num">${fmt(t.saldo_anterior)}</td><td class="num">${fmt(t.saldo_final)}</td>
      <td><span class="chip ${t.estado}">${t.estado}</span></td><td class="msg">${esc(t.mensaje)}</td></tr>`).join("");
}

function cabecera(conTitular) {
  return `<tr><th>ID</th><th>Fecha</th>${conTitular ? "<th>Titular</th>" : ""}<th>Tipo</th><th>Monto</th>
          <th>Saldo antes</th><th>Saldo después</th><th>Estado</th><th>Mensaje</th></tr>`;
}

async function cargarRecientes() {
  if (!seleccionada) return;
  const { datos } = await api("/api/transacciones?titular=" + encodeURIComponent(seleccionada));
  $("#tabla-recientes").innerHTML = cabecera(false) + filas(datos.slice(0, 6), false);
}

async function cargarHistorial() {
  const t = $("#filtro-titular").value;
  const { datos } = await api("/api/transacciones" + (t ? "?titular=" + encodeURIComponent(t) : ""));
  $("#tabla-historial").innerHTML = cabecera(true) + filas(datos, true);
  const cuenta = (e) => datos.filter((x) => x.estado === e).length;
  $("#resumen-estados").innerHTML = `
    <div class="chip EXITOSA"><strong>${cuenta("EXITOSA")}</strong>exitosas</div>
    <div class="chip RECHAZADA"><strong>${cuenta("RECHAZADA")}</strong>rechazadas</div>
    <div class="chip ERROR"><strong>${cuenta("ERROR")}</strong>con error</div>`;
}
$("#filtro-titular").addEventListener("change", cargarHistorial);

// ---------------- BDD ----------------
async function correrBDD() {
  const btn = $("#btn-bdd");
  btn.disabled = true;
  btn.textContent = "Ejecutando...";
  const { ok, datos } = await api("/api/bdd");
  btn.disabled = false;
  btn.textContent = "Ejecutar pruebas";
  if (!ok) { $("#bdd-resumen").textContent = datos.error; return; }
  const bien = datos.pasaron === datos.total;
  $("#bdd-resumen").innerHTML = `<span class="bdd-total ${bien ? "" : "mal"}">${datos.pasaron} de ${datos.total} escenarios pasaron</span>
     <span class="ayuda"> · ${datos.pasos_ok}/${datos.pasos_total} pasos OK</span>`;
  let html = "", feat = "";
  for (const e of datos.escenarios) {
    if (e.feature !== feat) { feat = e.feature; html += `<div class="feature-titulo">Feature: ${esc(feat)}</div>`; }
    html += `<details class="escenario" ${e.paso ? "" : "open"}>
      <summary><span>Scenario: ${esc(e.nombre)}</span><span class="chip ${e.paso ? "OK" : "FALLA"}">${e.paso ? "PASÓ" : "FALLÓ"}</span></summary>
      <ul>${e.pasos.map((p) => {
        const [kw, ...resto] = p.texto.split(" ");
        return `<li><span class="chip ${p.estado}">${p.estado}</span> <span class="kw">${kw}</span> ${esc(resto.join(" "))}
                ${p.error ? `<span class="err">→ ${esc(p.error)}</span>` : ""}</li>`;
      }).join("")}</ul></details>`;
  }
  $("#bdd-lista").innerHTML = html;
}
$("#btn-bdd").addEventListener("click", correrBDD);

// ---------------- pestañas ----------------
$$(".pestana").forEach((b) =>
  b.addEventListener("click", () => {
    $$(".pestana").forEach((x) => x.classList.toggle("activa", x === b));
    $$(".vista").forEach((v) => v.classList.toggle("activa", v.id === "vista-" + b.dataset.vista));
    if (b.dataset.vista === "historial") cargarHistorial();
    if (b.dataset.vista === "bdd" && !$("#bdd-lista").innerHTML) correrBDD();
  })
);

cargarCuentas().then(cargarRecientes);
