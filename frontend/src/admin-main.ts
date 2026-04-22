import { createApp, watch } from "vue";
import { createPinia } from "pinia";
import ArcoVue from "@arco-design/web-vue";
import router from "./router";
import App from "./App.vue";
import "./styles/theme.css";
import "./admin.css";
import "vditor/dist/index.css";

const app = createApp(App);
const pinia = createPinia();

app.use(pinia);
app.use(router);
app.use(ArcoVue);
app.mount("#admin-app");

watch(
  () => window.location.hash,
  () => {
    document.title = "Snemc Blog Admin";
  },
  { immediate: true },
);
