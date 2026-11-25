// Orbe do meia-noite. WebGL2 puro, sem biblioteca.
// Camadas: núcleo (esfera escura com luz laranja por baixo e azul por cima),
// casca de partículas, coroa, dois anéis inclinados e satélites (um por tarefa rodando).
"use strict";

// Cada estado é um conjunto de alvos; o orbe desliza de um para o outro.
const ESTADOS = {
  parado:      { rotulo: "parado",                 energia: 0.12, giro: 0.25, torcao: 0.0, pulso: 0.00, brilho: 0.85, corA: [0.30, 0.38, 1.00], corB: [1.00, 0.55, 0.18] },
  ouvindo:     { rotulo: "ouvindo",                energia: 0.38, giro: 0.60, torcao: 0.0, pulso: 0.30, brilho: 1.05, corA: [0.25, 0.70, 1.00], corB: [0.55, 0.50, 1.00] },
  pensando:    { rotulo: "pensando",               energia: 0.55, giro: 2.20, torcao: 1.3, pulso: 0.12, brilho: 1.00, corA: [0.66, 0.32, 1.00], corB: [1.00, 0.48, 0.30] },
  falando:     { rotulo: "falando",                energia: 0.45, giro: 0.80, torcao: 0.0, pulso: 1.00, brilho: 1.20, corA: [0.36, 0.42, 1.00], corB: [1.00, 0.62, 0.15] },
  trabalhando: { rotulo: "IAs trabalhando",        energia: 0.25, giro: 1.00, torcao: 0.3, pulso: 0.15, brilho: 0.95, corA: [0.30, 0.45, 1.00], corB: [1.00, 0.58, 0.20] },
  erro:        { rotulo: "algo deu errado",        energia: 0.70, giro: 0.30, torcao: 0.0, pulso: 0.55, brilho: 1.10, corA: [1.00, 0.22, 0.30], corB: [1.00, 0.30, 0.10] },
};
const ORDEM = ["parado", "ouvindo", "pensando", "falando", "trabalhando", "erro"];

const VERT_NUCLEO = `#version 300 es
void main() {
  // triângulo que cobre a tela inteira
  vec2 p = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2));
  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}`;

const FRAG_NUCLEO = `#version 300 es
precision highp float;
uniform vec2 uRes;
uniform float uEscala, uPulso, uBrilho;
uniform vec3 uCorA, uCorB;
out vec4 o;
void main() {
  vec2 q = (gl_FragCoord.xy * 2.0 - uRes) / uRes.y / uEscala;
  const float RAIO = 0.43;
  float r = length(q) / RAIO;
  vec3 cor = vec3(0.0);
  float alfa = 0.0;
  if (r < 1.0) {
    vec3 n = vec3(q / RAIO, sqrt(1.0 - r * r));
    float baixo = pow(max(dot(n, normalize(vec3(-0.30, -0.90, 0.22))), 0.0), 3.6);
    float cima  = pow(max(dot(n, normalize(vec3( 0.50,  0.80, 0.18))), 0.0), 5.0);
    float lado  = pow(max(dot(n, normalize(vec3(-0.95,  0.10, 0.15))), 0.0), 7.0);
    cor = vec3(0.030, 0.015, 0.070)
        + uCorB * baixo * (1.35 + uPulso * 1.0) * uBrilho
        + uCorA * cima * 1.15 * uBrilho
        + vec3(0.60, 0.15, 0.90) * lado * 0.55;
    alfa = smoothstep(1.0, 0.985, r);
    cor *= alfa;
  } else {
    float g = exp(-(r - 1.0) * 3.4);
    cor = (uCorB * (0.22 + uPulso * 0.35) + uCorA * 0.10) * g * uBrilho;
  }
  o = vec4(cor, alfa);
}`;

const VERT_PONTOS = `#version 300 es
precision highp float;
in vec3 aDir;
in float aSemente;
in float aTipo;
uniform float uT, uEnergia, uGiro, uTorcao, uPulso, uAspecto, uEscala, uPx, uSat, uFase;
uniform vec3 uCorA, uCorB;
out vec4 vCor;

mat3 rotX(float a) { float c = cos(a), s = sin(a); return mat3(1.0, 0.0, 0.0, 0.0, c, s, 0.0, -s, c); }
mat3 rotY(float a) { float c = cos(a), s = sin(a); return mat3(c, 0.0, -s, 0.0, 1.0, 0.0, s, 0.0, c); }
mat3 rotZ(float a) { float c = cos(a), s = sin(a); return mat3(c, s, 0.0, -s, c, 0.0, 0.0, 0.0, 1.0); }

void main() {
  vec3 p;
  vec3 cor;
  float alfa = 1.0;
  float tam = 1.0;

  if (aTipo < 0.5) {                       // casca de partículas
    vec3 d = aDir;
    float ang = uTorcao * d.y * 1.5;
    d = vec3(cos(ang) * d.x - sin(ang) * d.z, d.y, sin(ang) * d.x + cos(ang) * d.z);
    float n = sin(d.x * 6.0 + uT * 1.3 + aSemente * 6.283) * sin(d.y * 7.0 - uT * 1.1) * sin(d.z * 5.0 + uT * 0.9);
    float r = 1.0 + uEnergia * 0.20 * n + uPulso * 0.07 * sin(uT * 9.0 + d.y * 10.0);
    p = rotY(uFase) * (d * r);
    cor = mix(uCorA, uCorB, smoothstep(-0.6, 0.9, -d.y + n * 0.4));
    alfa = 0.50;
    tam = 1.5;
  } else if (aTipo < 1.5) {                // coroa: fiapos para fora
    float k = fract(aSemente * 17.3);
    float r = 1.06 + k * k * (0.34 + uEnergia * 0.55) * (0.6 + 0.4 * sin(uT * 2.0 + aSemente * 40.0));
    p = rotY(uFase * 0.6) * (aDir * r);
    cor = mix(uCorA, uCorB, smoothstep(-0.5, 0.8, -aDir.y));
    alfa = (1.0 - k) * 0.42;
    tam = 1.15;
  } else if (aTipo < 3.5) {                // anéis: aDir.x guarda o ângulo
    bool primeiro = aTipo < 2.5;
    float a = aDir.x + uT * uGiro * (primeiro ? 1.0 : -0.7);
    float R = (primeiro ? 1.50 : 1.74) + (aSemente - 0.5) * 0.035;
    p = vec3(cos(a) * R, (aSemente - 0.5) * 0.025, sin(a) * R);
    p = (primeiro ? rotZ(0.45) * rotX(0.38) : rotZ(-0.32) * rotX(-0.62)) * p;
    cor = mix(uCorB, vec3(0.95, 0.30, 0.55), 0.55);
    alfa = 0.30 + 0.25 * step(0.97, fract(aSemente * 7.7 + uT * 0.05));
    tam = 1.25;
  } else {                                 // satélites: aDir.x guarda o índice
    float i = aDir.x;
    float a = uT * (0.9 + 0.13 * i) + i * 2.4;
    float R = 1.42 + 0.07 * i;
    p = rotZ(0.5 * sin(i * 1.7)) * vec3(cos(a) * R, sin(a * 1.3 + i) * 0.32, sin(a) * R);
    cor = vec3(1.0, 0.66, 0.24);
    alfa = step(i + 0.5, uSat);
    tam = 7.5;
  }

  // o que passa por trás do núcleo quase some
  float sobreNucleo = 1.0 - smoothstep(0.70, 0.86, length(p.xy));
  if (aTipo < 1.5) alfa *= mix(1.0, p.z > 0.0 ? 0.05 : 0.22, sobreNucleo);
  else if (p.z > 0.0) alfa *= mix(1.0, 0.10, sobreNucleo);

  float z = p.z + 4.2;
  vec2 xy = p.xy / z * 2.3 * uEscala;
  xy.x /= uAspecto;
  gl_Position = vec4(xy, 0.0, 1.0);
  gl_PointSize = tam * uPx * uEscala * (4.2 / z);
  float perto = smoothstep(5.6, 3.0, z);
  vCor = vec4(cor, alfa * (0.30 + 0.70 * perto));
}`;

const FRAG_PONTOS = `#version 300 es
precision highp float;
in vec4 vCor;
out vec4 o;
void main() {
  float d = length(gl_PointCoord - 0.5);
  float a = smoothstep(0.5, 0.0, d) * vCor.a;
  o = vec4(vCor.rgb * a, a);
}`;

function compilar(gl, tipo, fonte) {
  const s = gl.createShader(tipo);
  gl.shaderSource(s, fonte);
  gl.compileShader(s);
  if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) {
    throw new Error("shader: " + gl.getShaderInfoLog(s));
  }
  return s;
}

function programa(gl, vert, frag) {
  const p = gl.createProgram();
  gl.attachShader(p, compilar(gl, gl.VERTEX_SHADER, vert));
  gl.attachShader(p, compilar(gl, gl.FRAGMENT_SHADER, frag));
  gl.linkProgram(p);
  if (!gl.getProgramParameter(p, gl.LINK_STATUS)) {
    throw new Error("programa: " + gl.getProgramInfoLog(p));
  }
  const u = {};
  const n = gl.getProgramParameter(p, gl.ACTIVE_UNIFORMS);
  for (let i = 0; i < n; i++) {
    const nome = gl.getActiveUniform(p, i).name;
    u[nome] = gl.getUniformLocation(p, nome);
  }
  return { p, u };
}

// Pontos: [dir.x, dir.y, dir.z, semente, tipo]
function montarPontos() {
  const dados = [];
  const aleatorio = (() => { let s = 1234567; return () => (s = (s * 1664525 + 1013904223) >>> 0) / 4294967296; })();
  const esfera = (n, tipo) => {
    const dourado = Math.PI * (3 - Math.sqrt(5));
    for (let i = 0; i < n; i++) {
      const y = 1 - (i / (n - 1)) * 2;
      const raio = Math.sqrt(1 - y * y);
      const a = dourado * i;
      let x = Math.cos(a) * raio + (aleatorio() - 0.5) * 0.05;
      let yy = y + (aleatorio() - 0.5) * 0.05;
      let z = Math.sin(a) * raio + (aleatorio() - 0.5) * 0.05;
      const m = Math.hypot(x, yy, z) || 1;
      dados.push(x / m, yy / m, z / m, aleatorio(), tipo);
    }
  };
  esfera(7000, 0);
  esfera(5200, 1);
  for (const tipo of [2, 3]) {
    const n = 1500;
    for (let i = 0; i < n; i++) dados.push((i / n) * Math.PI * 2, 0, 0, aleatorio(), tipo);
  }
  for (let i = 0; i < 8; i++) dados.push(i, 0, 0, aleatorio(), 4);
  return new Float32Array(dados);
}

function iniciar() {
  const tela = document.getElementById("orbe");
  const rotulo = document.getElementById("estado");
  const dica = document.getElementById("dica");
  const elVoce = document.getElementById("voce");
  const elResposta = document.getElementById("resposta");
  let temVoz = false;
  let dicaParado = "";

  function aplicarDica() {
    let texto = "";
    if (temVoz) {
      if (nomeEstado === "ouvindo") texto = "fale agora — Enter encerra";
      else if (nomeEstado === "falando") texto = "Enter interrompe";
      else if (nomeEstado === "pensando") texto = "";
      else texto = dicaParado || "aperte Enter ou clique para falar";
    }
    if (dica.textContent !== texto) dica.textContent = texto;
  }

  async function tocar() {
    if (!temVoz) return;
    try {
      await fetch("tocar", { method: "POST", headers: { "X-Meia-Noite": "1" }, cache: "no-store" });
      consultar();
    } catch (_) {
      dica.textContent = "o meia-noite não respondeu; rode mn rosto de novo";
    }
  }
  const gl = tela.getContext("webgl2", { alpha: true, premultipliedAlpha: true, antialias: true });
  if (!gl) {
    document.getElementById("aviso").hidden = false;
    return;
  }

  const nucleo = programa(gl, VERT_NUCLEO, FRAG_NUCLEO);
  const pontos = programa(gl, VERT_PONTOS, FRAG_PONTOS);

  const dados = montarPontos();
  const total = dados.length / 5;
  const vao = gl.createVertexArray();
  gl.bindVertexArray(vao);
  gl.bindBuffer(gl.ARRAY_BUFFER, gl.createBuffer());
  gl.bufferData(gl.ARRAY_BUFFER, dados, gl.STATIC_DRAW);
  const atributo = (nome, tamanho, deslocamento) => {
    const loc = gl.getAttribLocation(pontos.p, nome);
    gl.enableVertexAttribArray(loc);
    gl.vertexAttribPointer(loc, tamanho, gl.FLOAT, false, 20, deslocamento);
  };
  atributo("aDir", 3, 0);
  atributo("aSemente", 1, 12);
  atributo("aTipo", 1, 16);
  const vaoVazio = gl.createVertexArray();

  const consulta = new URLSearchParams(location.search);
  const fixo = ESTADOS[consulta.get("estado")] ? consulta.get("estado") : null;
  let nomeEstado = fixo || "parado";
  let satelites = Number(consulta.get("tarefas")) || 0;
  let forcadoAte = 0;

  // valores correntes, que perseguem o alvo do estado
  const inicial = ESTADOS[nomeEstado];
  const atual = { ...inicial, corA: [...inicial.corA], corB: [...inicial.corB], sat: satelites };
  let fase = 0;
  let anterior = performance.now();
  const calmo = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  function aplicarRotulo() {
    let texto = ESTADOS[nomeEstado].rotulo;
    if (nomeEstado === "trabalhando" && satelites > 0) {
      texto = satelites === 1 ? "1 IA trabalhando" : satelites + " IAs trabalhando";
    }
    if (rotulo.textContent !== texto) rotulo.textContent = texto;
  }

  function definir(nome, tarefas) {
    if (!ESTADOS[nome]) return;
    nomeEstado = nome;
    if (typeof tarefas === "number") satelites = Math.max(0, Math.min(8, tarefas));
    aplicarRotulo();
  }

  async function consultar() {
    if (fixo || performance.now() < forcadoAte) return;
    try {
      const r = await fetch("estado", { cache: "no-store" });
      if (!r.ok) return;
      const e = await r.json();
      temVoz = Boolean(e.voz);
      dicaParado = e.dica || "";
      definir(e.estado, e.rodando);
      if (elVoce.textContent !== (e.voce || "")) elVoce.textContent = e.voce || "";
      if (elResposta.textContent !== (e.resposta || "")) elResposta.textContent = e.resposta || "";
      aplicarDica();
    } catch (_) {
      // servidor fora do ar: mantém o último estado
    }
  }

  // teclas 1 a 6 mostram cada estado por alguns segundos (para ver e ajustar)
  window.addEventListener("keydown", (ev) => {
    if ((ev.key === "Enter" || ev.key === " ") && !ev.repeat) {
      ev.preventDefault();
      tocar();
      return;
    }
    const i = Number(ev.key) - 1;
    if (i >= 0 && i < ORDEM.length) {
      forcadoAte = performance.now() + 6000;
      definir(ORDEM[i], ORDEM[i] === "trabalhando" ? 3 : 0);
    }
  });

  function quadro(agora) {
    const dt = Math.min(0.05, (agora - anterior) / 1000);
    anterior = agora;
    const t = agora / 1000;

    const alvo = ESTADOS[nomeEstado];
    const k = 1 - Math.exp(-dt * 3.5);
    for (const c of ["energia", "giro", "torcao", "pulso", "brilho"]) atual[c] += (alvo[c] - atual[c]) * k;
    for (let i = 0; i < 3; i++) {
      atual.corA[i] += (alvo.corA[i] - atual.corA[i]) * k;
      atual.corB[i] += (alvo.corB[i] - atual.corB[i]) * k;
    }
    atual.sat += (satelites - atual.sat) * k;
    const ritmo = calmo ? 0.25 : 1;
    fase += dt * (0.12 + atual.giro * 0.10) * ritmo;

    // "voz" simulada enquanto não há áudio de verdade ligado
    const voz = 0.5 + 0.5 * Math.abs(Math.sin(t * 7.0) * Math.sin(t * 2.3 + 1.0));
    const pulso = atual.pulso * (nomeEstado === "falando" ? voz : 0.6 + 0.4 * Math.sin(t * 2.2));

    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    const w = Math.round(tela.clientWidth * dpr);
    const h = Math.round(tela.clientHeight * dpr);
    if (tela.width !== w || tela.height !== h) {
      tela.width = w;
      tela.height = h;
    }
    const aspecto = w / h;
    const escala = Math.min(1, aspecto) * 0.86;
    gl.viewport(0, 0, w, h);
    gl.clearColor(0, 0, 0, 0);
    gl.clear(gl.COLOR_BUFFER_BIT);
    gl.enable(gl.BLEND);

    gl.useProgram(nucleo.p);
    gl.bindVertexArray(vaoVazio);
    gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
    gl.uniform2f(nucleo.u.uRes, w, h);
    gl.uniform1f(nucleo.u.uEscala, escala);
    gl.uniform1f(nucleo.u.uPulso, pulso);
    gl.uniform1f(nucleo.u.uBrilho, atual.brilho);
    gl.uniform3fv(nucleo.u.uCorA, atual.corA);
    gl.uniform3fv(nucleo.u.uCorB, atual.corB);
    gl.drawArrays(gl.TRIANGLES, 0, 3);

    gl.useProgram(pontos.p);
    gl.bindVertexArray(vao);
    gl.blendFunc(gl.ONE, gl.ONE);
    gl.uniform1f(pontos.u.uT, t * ritmo);
    gl.uniform1f(pontos.u.uFase, fase);
    gl.uniform1f(pontos.u.uEnergia, atual.energia);
    gl.uniform1f(pontos.u.uGiro, atual.giro);
    gl.uniform1f(pontos.u.uTorcao, atual.torcao);
    gl.uniform1f(pontos.u.uPulso, pulso);
    gl.uniform1f(pontos.u.uAspecto, aspecto);
    gl.uniform1f(pontos.u.uEscala, escala);
    gl.uniform1f(pontos.u.uPx, (h / 520) * 1.6);
    gl.uniform1f(pontos.u.uSat, atual.sat);
    gl.uniform3fv(pontos.u.uCorA, atual.corA);
    gl.uniform3fv(pontos.u.uCorB, atual.corB);
    gl.drawArrays(gl.POINTS, 0, total);

    requestAnimationFrame(quadro);
  }

  tela.addEventListener("click", tocar);
  aplicarRotulo();
  consultar();
  setInterval(consultar, 500);
  requestAnimationFrame(quadro);
}

try {
  iniciar();
} catch (erro) {
  console.error(erro);
  const aviso = document.getElementById("aviso");
  aviso.textContent = "Não foi possível desenhar o orbe: " + erro.message;
  aviso.hidden = false;
}
